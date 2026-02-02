package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/metrics"
	"github.com/flowd-org/flowd/internal/observability/tracing"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sse"
)

type EventSink interface {
	Publish(runID string, ev sse.Event)
}

type EventFeed interface {
	Subscribe(ctx context.Context, runID, lastEventID string) *sse.Subscription
}

type EventSinkFunc func(runID string, ev sse.Event)

func (f EventSinkFunc) Publish(runID string, ev sse.Event) {
	f(runID, ev)
}

// NewRunEventsHandler streams events for GET /runs/{id}/events` using the Core DB
// journal for replay and the SSE Hub for live fan-out.
func NewRunEventsHandler(store *runstore.Store, hub EventFeed, journal *coredb.Journal) http.Handler {
	if store == nil {
		store = runstore.New()
	}
	_ = store
	if hub == nil {
		hub = sse.New(sse.Config{})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/events") {
			return
		}

		runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/runs/"), "/events")
		if runID == "" || strings.Contains(runID, "/") {
			response.Write(w, response.New(http.StatusNotFound, "run not found"))
			return
		}

		lastEventID := r.Header.Get("Last-Event-ID")
		if lastEventID == "" {
			lastEventID = r.URL.Query().Get("last_event_id")
		}
		lastSeq, err := coredb.ParseEventID(lastEventID)
		if err != nil {
			response.Write(w, response.New(http.StatusBadRequest, "invalid Last-Event-ID", response.WithDetail(err.Error())))
			return
		}
		resumeRequested := strings.TrimSpace(lastEventID) != ""

		ctx := r.Context()
		if resumeRequested {
			metrics.RecordSSEResumeAttempt()
		}

		if journal != nil && resumeRequested {
			var resumeErr error
			resumeOutcome := "ok"
			ctx, resumeSpan := tracing.Start(ctx, "server.sse.resume",
				tracing.RunID(runID),
				tracing.PersistDriver("sqlite"),
				tracing.PersistOp("resume"),
				tracing.PersistKeyspace("core_run_journal"),
				tracing.Int64("sse.last_seq", lastSeq),
			)
			defer func() {
				if resumeSpan != nil {
					resumeSpan.SetAttributes(tracing.String("sse.resume.outcome", resumeOutcome))
					tracing.End(resumeSpan, &resumeErr)
				}
			}()

			earliest, latest, boundsErr := journal.Bounds(ctx, runID)
			if boundsErr != nil {
				resumeOutcome = "error"
				resumeErr = boundsErr
				response.Write(w, response.New(http.StatusInternalServerError, "journal lookup failed"))
				return
			}
			if earliest == 0 {
				resumeOutcome = "expired"
				resumeErr = fmt.Errorf("cursor %d expired", lastSeq)
				metrics.RecordSSECursorExpired()
				response.Write(w, response.New(http.StatusGone, "cursor expired",
					response.WithType("https://flowd.org/problems/sse/stale-cursor"),
					response.WithDetail("run events are no longer retained"),
				))
				return
			}
			if latest > 0 && lastSeq > latest {
				resumeOutcome = "invalid"
				resumeErr = fmt.Errorf("cursor %d beyond latest %d", lastSeq, latest)
				response.Write(w, response.New(http.StatusBadRequest, "invalid Last-Event-ID",
					response.WithDetail(fmt.Sprintf("cursor %d beyond latest %d", lastSeq, latest)),
				))
				return
			}
			if lastSeq < earliest {
				resumeOutcome = "expired"
				resumeErr = fmt.Errorf("cursor %d expired", lastSeq)
				metrics.RecordSSECursorExpired()
				response.Write(w, response.New(http.StatusGone, "cursor expired",
					response.WithType("https://flowd.org/problems/sse/stale-cursor"),
					response.WithDetail(fmt.Sprintf("cursor %d no longer retained", lastSeq)),
				))
				return
			}
		}

		endStream := metrics.SSEStreamStarted()
		defer func() {
			if endStream != nil {
				endStream()
			}
		}()

		sub := hub.Subscribe(ctx, runID, lastEventID)
		defer sub.Close()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte("retry: 3000\n")); err != nil {
			return
		}
		if _, err := w.Write(sse.FormatHeartbeat(time.Now().UTC())); err != nil {
			return
		}
		flush(w)

		lastSentSeq := lastSeq
		if journal != nil {
			err = journal.ForEach(ctx, runID, lastSeq, func(entry coredb.JournalEntry) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				payload, err := sse.EncodeEvent(sse.Event{
					ID:        fmt.Sprintf("%d", entry.Seq),
					Event:     entry.EventType,
					Data:      string(entry.Payload),
					Timestamp: entry.Timestamp,
					RunID:     runID,
				})
				if err != nil {
					return err
				}
				if err := writeSSEPayload(ctx, w, runID, payload, entry.Seq); err != nil {
					return err
				}
				lastSentSeq = entry.Seq
				return nil
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				// Streaming has already begun; the best we can do is abort.
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.C:
				if !ok {
					return
				}
				msgSeq := extractEventID(msg)
				if msgSeq > 0 && msgSeq <= lastSentSeq {
					continue
				}
				if msgSeq > lastSentSeq {
					lastSentSeq = msgSeq
				}
				if err := writeSSEPayload(ctx, w, runID, msg, msgSeq); err != nil {
					return
				}
			}
		}
	})
}

func writeSSE(ctx context.Context, w http.ResponseWriter, runID string, ev sse.Event) (err error) {
	_, span := tracing.Start(ctx, "server.sse.write",
		tracing.RunID(runID),
		tracing.PersistDriver("sqlite"),
		tracing.PersistOp("write"),
		tracing.PersistKeyspace("core_run_journal"),
	)
	defer tracing.End(span, &err)
	if span != nil {
		span.SetAttributes(tracing.String("sse.event_type", ev.Event))
		if seq, parseErr := coredb.ParseEventID(ev.ID); parseErr == nil && seq > 0 {
			span.SetAttributes(tracing.Int64("sse.event_seq", seq))
		}
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	payload, err := sse.EncodeEvent(ev)
	if err != nil {
		return err
	}
	if _, err = w.Write(payload); err != nil {
		return err
	}
	flush(w)
	return nil
}

func writeSSEPayload(ctx context.Context, w http.ResponseWriter, runID string, payload []byte, seq int64) (err error) {
	_, span := tracing.Start(ctx, "server.sse.write",
		tracing.RunID(runID),
		tracing.PersistDriver("sqlite"),
		tracing.PersistOp("write"),
		tracing.PersistKeyspace("core_run_journal"),
	)
	if span != nil && seq > 0 {
		span.SetAttributes(tracing.Int64("sse.event_seq", seq))
	}
	defer tracing.End(span, &err)
	if _, err = w.Write(payload); err != nil {
		return err
	}
	flush(w)
	return nil
}

func flush(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func extractEventID(msg []byte) int64 {
	lines := strings.Split(string(msg), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "id:") {
			id := strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			seq, err := coredb.ParseEventID(id)
			if err == nil {
				return seq
			}
			return 0
		}
		if line == "" {
			break
		}
	}
	return 0
}

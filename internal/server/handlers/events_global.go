package handlers

import (
	"context"
	"encoding/json"
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

// EventsConfig configures the global events handler.
type EventsConfig struct {
	RunStore  *runstore.Store
	RunHub    *sse.Hub
	GlobalHub *sse.Hub
	Journal   *coredb.Journal
}

// NewEventsHandler returns an SSE handler for global events streams (GET /events/stream, GET /events).
func NewEventsHandler(cfg EventsConfig) http.Handler {
	store := cfg.RunStore
	if store == nil {
		store = runstore.New()
	}
	runHub := cfg.RunHub
	if runHub == nil {
		runHub = sse.New(sse.Config{})
	}
	globalHub := cfg.GlobalHub
	if globalHub == nil {
		globalHub = sse.New(sse.Config{})
	}
	journal := cfg.Journal

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
			return
		}

		runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
		lastEventID := r.Header.Get("Last-Event-ID")
		if lastEventID == "" {
			lastEventID = r.URL.Query().Get("last_event_id")
		}
		var hub *sse.Hub
		contextID := "global"
		resumeRequested := strings.TrimSpace(lastEventID) != ""

		if runID != "" {
			if _, ok := store.Get(runID); !ok {
				response.Write(w, response.New(http.StatusNotFound, "run not found", response.WithDetail(runID)))
				return
			}
			hub = runHub
			contextID = runID
		} else {
			hub = globalHub
		}

		ctx := r.Context()
		if resumeRequested {
			metrics.RecordSSEResumeAttempt()
		}

		var lastSeq int64
		if resumeRequested {
			parsedSeq, err := coredb.ParseEventID(lastEventID)
			if err != nil {
				response.Write(w, response.New(http.StatusBadRequest, "invalid Last-Event-ID", response.WithDetail(err.Error())))
				return
			}
			lastSeq = parsedSeq
		}

		if journal != nil && resumeRequested {
			var resumeErr error
			resumeOutcome := "ok"
			ctx, resumeSpan := tracing.Start(ctx, "server.sse.resume",
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

			var earliest, latest int64
			if runID != "" {
				earliest, latest, resumeErr = journal.Bounds(ctx, runID)
			} else {
				earliest, latest, resumeErr = journal.BoundsAll(ctx)
			}
			if resumeErr != nil {
				resumeOutcome = "error"
				response.Write(w, response.New(http.StatusInternalServerError, "journal lookup failed"))
				return
			}
			if earliest == 0 {
				resumeOutcome = "expired"
				metrics.RecordSSECursorExpired()
				response.Write(w, response.New(http.StatusGone, "cursor expired",
					response.WithType("https://flowd.org/problems/sse/stale-cursor"),
					response.WithDetail("run events are no longer retained"),
				))
				return
			}
			if latest > 0 && lastSeq > latest {
				resumeOutcome = "invalid"
				response.Write(w, response.New(http.StatusBadRequest, "invalid Last-Event-ID",
					response.WithDetail(fmt.Sprintf("cursor %d beyond latest %d", lastSeq, latest)),
				))
				return
			}
			if lastSeq < earliest {
				resumeOutcome = "expired"
				metrics.RecordSSECursorExpired()
				response.Write(w, response.New(http.StatusGone, "cursor expired",
					response.WithType("https://flowd.org/problems/sse/stale-cursor"),
					response.WithDetail(fmt.Sprintf("cursor %d no longer retained", lastSeq)),
				))
				return
			}
		}
		sub := hub.Subscribe(ctx, contextID, lastEventID)
		defer sub.Close()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		// Explicitly send 200 headers, retry directive, and an initial heartbeat
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("retry: 3000\n"))
		_, _ = w.Write(sse.FormatHeartbeat(time.Now().UTC()))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		endStream := metrics.SSEStreamStarted()
		defer func() {
			if endStream != nil {
				endStream()
			}
		}()

		if journal != nil {
			var replayErr error
			if runID != "" {
				replayErr = journal.ForEach(ctx, runID, lastSeq, func(entry coredb.JournalEntry) error {
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
					lastSeq = entry.Seq
					return nil
				})
			} else {
				replayErr = journal.ForEachAll(ctx, lastSeq, func(entry coredb.JournalEntry) error {
					wrapped := WrapGlobalEvent(entry.RunID, sse.Event{
						ID:        fmt.Sprintf("%d", entry.Seq),
						Event:     entry.EventType,
						Data:      string(entry.Payload),
						Timestamp: entry.Timestamp,
					})
					payload, err := sse.EncodeEvent(wrapped)
					if err != nil {
						return err
					}
					if err := writeSSEPayload(ctx, w, entry.RunID, payload, entry.Seq); err != nil {
						return err
					}
					lastSeq = entry.Seq
					return nil
				})
			}
			if replayErr != nil && !errors.Is(replayErr, context.Canceled) {
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
				if msgSeq > 0 && msgSeq <= lastSeq {
					continue
				}
				if msgSeq > lastSeq {
					lastSeq = msgSeq
				}
				if err := writeSSEPayload(ctx, w, runID, msg, msgSeq); err != nil {
					return
				}
			}
		}
	})
}

// WrapGlobalEvent ensures run_id is present in the SSE payload (used by router).
func WrapGlobalEvent(runID string, ev sse.Event) sse.Event {
	if runID == "" {
		return ev
	}
	ev.RunID = runID
	var payload map[string]any
	if err := json.Unmarshal([]byte(ev.Data), &payload); err != nil || payload == nil {
		payload = map[string]any{
			"run_id": runID,
			"data":   ev.Data,
		}
	} else {
		payload["run_id"] = runID
	}
	data, err := json.Marshal(payload)
	if err == nil {
		ev.Data = string(data)
	}
	return ev
}

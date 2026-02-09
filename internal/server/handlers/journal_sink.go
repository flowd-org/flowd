package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/sse"
)

type journalEventSink struct {
	journal *coredb.Journal
	next    EventSink
	logger  *slog.Logger
}

// NewJournalEventSink returns an EventSink that persists run events in the Core
// DB journal before broadcasting them to the downstream sink (typically the SSE
// hub). When journal is nil the downstream sink is returned untouched.
func NewJournalEventSink(journal *coredb.Journal, next EventSink) EventSink {
	if journal == nil {
		return next
	}
	logger := slog.Default()
	return &journalEventSink{
		journal: journal,
		next:    next,
		logger:  logger,
	}
}

func (s *journalEventSink) Publish(runID string, ev sse.Event) {
	if s == nil {
		return
	}
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	data := ev.Data
	if data != "" {
		data = ensureJournalIdentityPayload(data, ev.Tenant)
	}
	entry, err := s.journal.Append(context.Background(), runID, ev.Event, []byte(data), ts)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("persist run event", slog.String("run_id", runID), slog.String("event", ev.Event), slog.String("error", err.Error()))
		}
		return
	}
	ev.ID = strconv.FormatInt(entry.Seq, 10)
	ev.Timestamp = entry.Timestamp
	if ev.RunID == "" {
		ev.RunID = runID
	}
	if s.next != nil {
		s.next.Publish(runID, ev)
	}
}

func ensureJournalIdentityPayload(data string, tenant string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil || len(payload) == 0 {
		return data
	}

	var (
		provenanceTenant string
		provenanceOrigin jobOrigin
		hasProvenance    bool
	)
	if provenance, ok := payload["provenance"].(map[string]any); ok {
		provenanceTenant, provenanceOrigin, hasProvenance = runIdentityFromProvenance(provenance)
	}

	if tenant != "" {
		payload["tenant"] = tenant
	} else if hasProvenance && provenanceTenant != "" {
		payload["tenant"] = provenanceTenant
	}

	if hasProvenance && (provenanceOrigin.SourceKind != "" || provenanceOrigin.SourceName != "") {
		payload["origin"] = provenanceOrigin
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return string(bytes)
}

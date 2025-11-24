package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/runstore"
)

const (
	exportExtensionName  = "export"
	extensionUnsupported = "about:blank#extension-unsupported"
	cursorExpiredProblem = "https://flowd.dev/problems/cursor-expired"
	ndjsonContentType    = "text/x-ndjson"
	exportCacheControl   = "no-store"
)

type runEventsExportHandler struct {
	store   *runstore.Store
	journal *coredb.Journal
	enabled bool
}

// NewRunEventsExportHandler streams journal events as NDJSON for GET /runs/{id}/events.ndjson.
func NewRunEventsExportHandler(store *runstore.Store, journal *coredb.Journal, enabled bool) http.Handler {
	return &runEventsExportHandler{
		store:   store,
		journal: journal,
		enabled: enabled,
	}
}

func (h *runEventsExportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/events.ndjson") {
		response.Write(w, response.New(http.StatusNotFound, "run not found"))
		return
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/runs/"), "/events.ndjson")
	runID = strings.Trim(runID, "/")
	if runID == "" {
		response.Write(w, response.New(http.StatusNotFound, "run not found"))
		return
	}

	if !h.enabled {
		response.Write(w, response.New(http.StatusNotFound, "extension unsupported",
			response.WithType(extensionUnsupported),
			response.WithExtension("extension", exportExtensionName),
		))
		return
	}

	if h.store != nil {
		if _, ok := h.store.Get(runID); !ok {
			response.Write(w, response.New(http.StatusNotFound, "run not found"))
			return
		}
	}

	if h.journal == nil {
		response.Write(w, response.New(http.StatusInternalServerError, "journal unavailable"))
		return
	}

	ctx := r.Context()
	earliest, _, err := h.journal.Bounds(ctx, runID)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "journal lookup failed", response.WithDetail(err.Error())))
		return
	}
	if earliest == 0 {
		response.Write(w, response.New(http.StatusGone, "cursor expired",
			response.WithType(cursorExpiredProblem),
			response.WithDetail("run events are no longer retained"),
		))
		return
	}

	w.Header().Set("Content-Type", ndjsonContentType)
	w.Header().Set("Cache-Control", exportCacheControl)

	writer := bufio.NewWriter(w)
	eventErr := h.journal.ForEach(ctx, runID, 0, func(entry coredb.JournalEntry) error {
		line := exportEvent{
			Sequence:  entry.Seq,
			Timestamp: entry.Timestamp,
			Event:     entry.EventType,
			Data:      json.RawMessage(entry.Payload),
		}
		payload, err := json.Marshal(line)
		if err != nil {
			return err
		}
		if _, err := writer.Write(payload); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
		return writer.Flush()
	})
	if eventErr != nil {
		if errors.Is(eventErr, context.Canceled) {
			return
		}
		response.Write(w, response.New(http.StatusInternalServerError, "journal read failed", response.WithDetail(eventErr.Error())))
		return
	}
}

type exportEvent struct {
	Sequence  int64           `json:"sequence"`
	Timestamp time.Time       `json:"timestamp"`
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
}

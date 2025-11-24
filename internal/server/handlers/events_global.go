package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sse"
)

// EventsConfig configures the global events handler.
type EventsConfig struct {
	RunStore  *runstore.Store
	RunHub    *sse.Hub
	GlobalHub *sse.Hub
}

// NewEventsHandler returns an SSE handler for GET /events.
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
		sub := hub.Subscribe(ctx, contextID, lastEventID)
		defer sub.Close()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		// Explicitly send 200 headers and an initial comment
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(":connected\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.C:
				if !ok {
					return
				}
				if _, err := w.Write(msg); err != nil {
					return
				}
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
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

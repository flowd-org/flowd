// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/flowd-org/flowd/internal/server/response"
)

const (
	limitsAlgorithmDefault        = "wfq"
	limitsQueueMaxDepthDefault    = 1024
	limitsBackpressureModeDefault = "reject_when_full"
	limitsConcurrencyMin          = 4
)

type limitsQueueStats struct {
	Len      int `json:"len"`
	Enqueued int `json:"enqueued"`
	Dequeued int `json:"dequeued"`
	Dropped  int `json:"dropped"`
}

type limitsResponse struct {
	Algorithm        string           `json:"algorithm"`
	Concurrency      int              `json:"concurrency"`
	QueueMaxDepth    int              `json:"queue_max_depth"`
	BackpressureMode string           `json:"backpressure_mode"`
	QueueStats       limitsQueueStats `json:"queue_stats"`
	UpdatedAt        string           `json:"updated_at"`
}

type limitsHandler struct {
	now func() time.Time
}

// NewLimitsHandler returns an HTTP handler for GET /limits.
func NewLimitsHandler() http.Handler {
	return &limitsHandler{
		now: time.Now,
	}
}

func (h *limitsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}

	payload := limitsResponse{
		Algorithm:        limitsAlgorithmDefault,
		Concurrency:      defaultConcurrency(runtime.GOMAXPROCS(0)),
		QueueMaxDepth:    limitsQueueMaxDepthDefault,
		BackpressureMode: limitsBackpressureModeDefault,
		QueueStats:       limitsQueueStats{},
		UpdatedAt:        h.now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func defaultConcurrency(gomaxprocs int) int {
	concurrency := 2 * gomaxprocs
	if concurrency < limitsConcurrencyMin {
		return limitsConcurrencyMin
	}
	return concurrency
}

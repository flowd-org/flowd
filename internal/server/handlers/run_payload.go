package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/runstore"
)

type RunPayload struct {
	ID              string         `json:"id"`
	JobID           string         `json:"job_id"`
	Status          string         `json:"status"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	Result          map[string]any `json:"result,omitempty"`
	Executor        string         `json:"executor,omitempty"`
	Runtime         string         `json:"runtime,omitempty"`
	SecurityProfile string         `json:"security_profile,omitempty"`
	Provenance      map[string]any `json:"provenance,omitempty"`
}

func newRunPayload(id, jobID, status string, startedAt time.Time) RunPayload {
	return RunPayload{
		ID:        id,
		JobID:     jobID,
		Status:    status,
		StartedAt: startedAt,
	}
}

func payloadFromStore(run runstore.Run) RunPayload {
	return RunPayload{
		ID:         run.ID,
		JobID:      run.JobID,
		Status:     run.Status,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
		Result:     run.Result,
		Executor:   run.Executor,
		Runtime:    run.Runtime,
		Provenance: run.Provenance,
	}
}

func writeRunPayload(w http.ResponseWriter, payload RunPayload, status int) {
	data, err := json.Marshal(payload)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "encode run failed", response.WithDetail(err.Error())))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

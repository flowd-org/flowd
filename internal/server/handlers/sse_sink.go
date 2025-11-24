package handlers

import (
	"encoding/json"
	"time"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/server/sse"
)

// newSSESink adapts the events.Sink interface to publish SSE events enriched with run metadata.
func newSSESink(sink EventSink, payload *RunPayload) events.Sink {
	if sink == nil {
		return nil
	}
	return &sseSink{
		sink: sink,
		run:  payload,
	}
}

type sseSink struct {
	sink EventSink
	run  *RunPayload
}

func (s *sseSink) EmitRunStart(runID, jobID string) {
	data := s.basePayload()
	data["status"] = "running"
	if !s.run.StartedAt.IsZero() {
		data["started_at"] = s.run.StartedAt
	}
	s.publish("run.start", data)
}

func (s *sseSink) EmitRunFinish(runID, status string, err error) {
	data := s.basePayload()
	data["status"] = status
	if s.run.FinishedAt != nil {
		data["finished_at"] = s.run.FinishedAt
	} else {
		data["finished_at"] = time.Now().UTC()
	}
	if err != nil {
		data["error"] = err.Error()
	}
	s.publish("run.finish", data)
}

func (s *sseSink) EmitStepStart(runID, step string) {
	data := s.basePayload()
	data["step"] = step
	s.publish("step.start", data)
}

func (s *sseSink) EmitStepLog(runID, step, channel, message string) {
	data := s.basePayload()
	data["step"] = step
	data["channel"] = channel
	data["message"] = message
	s.publish("step.log", data)
}

func (s *sseSink) EmitStepFinish(runID, step string, exitCode int, err error) {
	data := s.basePayload()
	data["step"] = step
	data["exit_code"] = exitCode
	if err != nil {
		data["error"] = err.Error()
		data["status"] = "failed"
	} else {
		data["status"] = "completed"
	}
	s.publish("step.finish", data)
}

func (s *sseSink) basePayload() map[string]any {
	payload := map[string]any{}
	if s.run != nil {
		payload["run_id"] = s.run.ID
		payload["job_id"] = s.run.JobID
		if !s.run.StartedAt.IsZero() {
			payload["started_at"] = s.run.StartedAt
		}
		if s.run.Executor != "" {
			payload["executor"] = s.run.Executor
		}
		if s.run.Runtime != "" {
			payload["runtime"] = s.run.Runtime
		}
		if s.run.Provenance != nil {
			payload["provenance"] = s.run.Provenance
		}
		if s.run.FinishedAt != nil {
			payload["finished_at"] = s.run.FinishedAt
		}
	}
	return payload
}

func (s *sseSink) publish(event string, payload map[string]any) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		bytes = []byte("{}")
	}
	s.sink.Publish(s.run.ID, sse.Event{Event: event, Data: string(bytes)})
}

// SPDX-License-Identifier: AGPL-3.0-or-later
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
)

type JournalSink struct {
	journal *coredb.Journal
	nowFn   func() time.Time
}

func NewJournalSink(journal *coredb.Journal) *JournalSink {
	if journal == nil {
		return nil
	}
	return &JournalSink{
		journal: journal,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *JournalSink) EmitRunStart(runID, jobID string) {
	payload := s.basePayload(runID)
	payload["job_id"] = jobID
	payload["status"] = "running"
	s.append(runID, TypeRunStart, payload)
}

func (s *JournalSink) EmitRunFinish(runID, status string, err error) {
	payload := s.basePayload(runID)
	payload["status"] = status
	if err != nil {
		payload["error"] = err.Error()
	}
	s.append(runID, TypeRunFinish, payload)
}

func (s *JournalSink) EmitStepStart(runID, step string) {
	payload := s.basePayload(runID)
	payload["step"] = step
	s.append(runID, TypeStepStart, payload)
}

func (s *JournalSink) EmitStepLog(runID, step, channel, message string) {
	if message == "" {
		return
	}
	payload := s.basePayload(runID)
	payload["step"] = step
	payload["channel"] = channel
	payload["message"] = message
	s.append(runID, TypeStepLog, payload)
}

func (s *JournalSink) EmitStepFinish(runID, step string, exitCode int, err error) {
	payload := s.basePayload(runID)
	payload["step"] = step
	payload["exit_code"] = exitCode
	status := "completed"
	if exitCode != 0 || err != nil {
		status = "failed"
	}
	payload["status"] = status
	if err != nil {
		payload["error"] = err.Error()
	}
	s.append(runID, TypeStepFinish, payload)
}

func (s *JournalSink) basePayload(runID string) map[string]any {
	payload := map[string]any{}
	if runID != "" {
		payload["run_id"] = runID
	}
	payload["timestamp"] = s.nowFn()
	return payload
}

func (s *JournalSink) append(runID, eventType string, payload map[string]any) {
	if s == nil || s.journal == nil {
		return
	}
	if runID == "" {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	ts, _ := payload["timestamp"].(time.Time)
	bytes, err := json.Marshal(payload)
	if err != nil {
		bytes = []byte("{}")
	}
	_, _ = s.journal.Append(context.Background(), runID, eventType, bytes, ts)
}

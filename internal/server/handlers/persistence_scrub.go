// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"encoding/json"
	"strings"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/secrets"
	"github.com/flowd-org/flowd/internal/server/sse"
)

type persistenceScrubber struct {
	secretNames       map[string]struct{}
	secretValues      []string
	secretHandlePaths []string
	baseDir           string
}

type scrubbedEventSink struct {
	next     EventSink
	scrubber *persistenceScrubber
}

func newScrubbedEventSink(next EventSink, scrubber *persistenceScrubber) EventSink {
	if next == nil || scrubber == nil {
		return next
	}
	return &scrubbedEventSink{next: next, scrubber: scrubber}
}

func (s *scrubbedEventSink) Publish(runID string, ev sse.Event) {
	if s == nil || s.next == nil {
		return
	}
	if s.scrubber != nil {
		ev.Data = scrubEventDataForPersistence(ev.Data, s.scrubber)
	}
	s.next.Publish(runID, ev)
}

func newPersistenceScrubber(binding *engine.Binding, secretHandles map[string]string) *persistenceScrubber {
	scrubber := &persistenceScrubber{baseDir: strings.TrimSpace(secrets.BaseDir())}
	if binding != nil {
		if len(binding.SecretNames) > 0 {
			scrubber.secretNames = binding.SecretNames
		}
		if len(binding.SecretValues) > 0 {
			scrubber.secretValues = append([]string{}, binding.SecretValues...)
		}
	}
	if len(secretHandles) > 0 {
		scrubber.secretHandlePaths = make([]string, 0, len(secretHandles))
		for _, path := range secretHandles {
			if strings.TrimSpace(path) == "" {
				continue
			}
			scrubber.secretHandlePaths = append(scrubber.secretHandlePaths, path)
		}
	}
	return scrubber
}

func scrubRunPayloadForPersistence(payload RunPayload, binding *engine.Binding, secretHandles map[string]string) RunPayload {
	scrubber := newPersistenceScrubber(binding, secretHandles)
	return scrubRunPayload(payload, scrubber)
}

func scrubRunRecordForPersistence(record coredb.RunRecord, binding *engine.Binding, secretHandles map[string]string) coredb.RunRecord {
	scrubber := newPersistenceScrubber(binding, secretHandles)
	return scrubRunRecord(record, scrubber)
}

func scrubEventDataForPersistence(data string, scrubber *persistenceScrubber) string {
	if data == "" {
		return data
	}
	if scrubber == nil {
		scrubber = newPersistenceScrubber(nil, nil)
	}
	var decoded any
	if err := json.Unmarshal([]byte(data), &decoded); err == nil {
		scrubbed := scrubAny(decoded, scrubber)
		if scrubbedBytes, err := json.Marshal(scrubbed); err == nil {
			return string(scrubbedBytes)
		}
	}
	if scrubber.shouldRedactString(data) {
		return events.SecretToken()
	}
	return data
}

func scrubRunPayload(payload RunPayload, scrubber *persistenceScrubber) RunPayload {
	out := payload
	if payload.Result != nil {
		out.Result = scrubMap(payload.Result, scrubber)
	}
	if payload.Provenance != nil {
		out.Provenance = scrubMap(payload.Provenance, scrubber)
	}
	return out
}

func scrubRunRecord(record coredb.RunRecord, scrubber *persistenceScrubber) coredb.RunRecord {
	out := record
	if record.Result != nil {
		out.Result = scrubMap(record.Result, scrubber)
	}
	if record.Provenance != nil {
		out.Provenance = scrubMap(record.Provenance, scrubber)
	}
	return out
}

func scrubMap(values map[string]any, scrubber *persistenceScrubber) map[string]any {
	if len(values) == 0 {
		return values
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		if scrubber != nil && scrubber.isSecretKey(key) {
			out[key] = events.SecretToken()
			continue
		}
		out[key] = scrubAny(value, scrubber)
	}
	return out
}

func scrubAny(value any, scrubber *persistenceScrubber) any {
	switch v := value.(type) {
	case map[string]any:
		return scrubMap(v, scrubber)
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, val := range v {
			if scrubber != nil && scrubber.isSecretKey(key) {
				out[key] = events.SecretToken()
				continue
			}
			out[key] = scrubString(val, scrubber)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = scrubAny(elem, scrubber)
		}
		return out
	case []string:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = scrubString(elem, scrubber)
		}
		return out
	case string:
		return scrubString(v, scrubber)
	default:
		return value
	}
}

func scrubString(value string, scrubber *persistenceScrubber) string {
	if scrubber == nil {
		return value
	}
	if scrubber.shouldRedactString(value) {
		return events.SecretToken()
	}
	return value
}

func (s *persistenceScrubber) isSecretKey(key string) bool {
	if s == nil || len(s.secretNames) == 0 {
		return false
	}
	_, ok := s.secretNames[key]
	return ok
}

func (s *persistenceScrubber) shouldRedactString(value string) bool {
	if s == nil {
		return false
	}
	if value == "" || value == events.SecretToken() {
		return false
	}
	for _, secret := range s.secretValues {
		if secret != "" && strings.Contains(value, secret) {
			return true
		}
	}
	for _, path := range s.secretHandlePaths {
		if path != "" && strings.Contains(value, path) {
			return true
		}
	}
	if s.baseDir != "" && strings.Contains(value, s.baseDir) {
		return true
	}
	return false
}

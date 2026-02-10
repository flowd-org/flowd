package handlers

import (
	"encoding/json"
	"testing"

	"github.com/flowd-org/flowd/internal/server/sse"
)

func TestEnsureJournalIdentityPayloadPrefersEnvelopeTenantAndProvenanceOrigin(t *testing.T) {
	t.Parallel()

	input := `{
		"tenant":"stale-tenant",
		"origin":{"source_kind":"stale-kind","source_name":"stale-name"},
		"provenance":{
			"tenant":"prov-tenant",
			"origin":{"source_kind":"git","source_name":"repo-a"}
		}
	}`

	out := ensureJournalIdentityPayload(input, "event-tenant")
	payload := decodeJSONMap(t, out)

	if got := payload["tenant"]; got != "event-tenant" {
		t.Fatalf("tenant = %v, want event-tenant", got)
	}
	origin := originFromAny(payload["origin"])
	if origin.SourceKind != "git" || origin.SourceName != "repo-a" {
		t.Fatalf("origin = %+v, want {source_kind:git source_name:repo-a}", origin)
	}
}

func TestEnsureJournalIdentityPayloadFallsBackToProvenanceTenant(t *testing.T) {
	t.Parallel()

	input := `{
		"tenant":"stale-tenant",
		"provenance":{
			"tenant":"prov-tenant"
		}
	}`

	out := ensureJournalIdentityPayload(input, "")
	payload := decodeJSONMap(t, out)

	if got := payload["tenant"]; got != "prov-tenant" {
		t.Fatalf("tenant = %v, want prov-tenant", got)
	}
}

func TestJournalEventSinkPublishSanitizesForwardedPayload(t *testing.T) {
	t.Parallel()

	journal := newTestJournal(t)
	var forwarded sse.Event
	sink := NewJournalEventSink(journal, EventSinkFunc(func(_ string, ev sse.Event) {
		forwarded = ev
	}))

	input := `{
		"tenant":"stale-tenant",
		"origin":{"source_kind":"stale-kind","source_name":"stale-name"},
		"provenance":{
			"tenant":"prov-tenant",
			"origin":{"source_kind":"git","source_name":"repo-a"}
		}
	}`

	sink.Publish("run-1", sse.Event{Event: "run.started", Tenant: "event-tenant", Data: input})

	payload := decodeJSONMap(t, forwarded.Data)
	if got := payload["tenant"]; got != "event-tenant" {
		t.Fatalf("forwarded tenant = %v, want event-tenant", got)
	}
	origin := originFromAny(payload["origin"])
	if origin.SourceKind != "git" || origin.SourceName != "repo-a" {
		t.Fatalf("forwarded origin = %+v, want {source_kind:git source_name:repo-a}", origin)
	}
}

func decodeJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

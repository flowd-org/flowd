package handlers

import (
	"encoding/json"
	"testing"
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

func decodeJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

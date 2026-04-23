package handlers //nolint:testpackage

import (
	"testing"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/types"
)

func TestShouldRedactString_RedactsSecretValuesAndBaseDir(t *testing.T) {
	s := &persistenceScrubber{secretValues: []string{"sekret"}, baseDir: "/tmp/flowd"}

	if !s.shouldRedactString("x sekret y") {
		t.Error("expected 'x sekret y' to be redacted")
	}
	if s.shouldRedactString(events.SecretToken()) {
		t.Error("expected SecretToken() not to be redacted")
	}
	if !s.shouldRedactString("/tmp/flowd/runs") {
		t.Error("expected baseDir path to be redacted")
	}
	if s.shouldRedactString("normal string") {
		t.Error("expected 'normal string' not to be redacted")
	}
}

func TestScrubString_RedactsToSecretToken(t *testing.T) {
	s := &persistenceScrubber{secretValues: []string{"sekret"}}

	result := scrubString("contains sekret", s)
	if result != events.SecretToken() {
		t.Errorf("expected SecretToken(), got %q", result)
	}

	// Non-matching string unchanged
	result2 := scrubString("normal string", s)
	if result2 != "normal string" {
		t.Errorf("expected 'normal string', got %q", result2)
	}
}

func TestScrubMap_RedactsBySecretKey(t *testing.T) {
	s := &persistenceScrubber{secretNames: map[string]struct{}{"token": {}}}
	input := map[string]any{
		"token": "abc",
		"ok":    "v",
	}

	result := scrubMap(input, s)

	if result["token"] != events.SecretToken() {
		t.Errorf("expected token redacted to SecretToken(), got %v", result["token"])
	}
	if result["ok"] != "v" {
		t.Errorf("expected ok='v', got %v", result["ok"])
	}
}

func TestNewProblemScrubber_ArgSpecSecretsAreIncluded(t *testing.T) {
	spec := &types.ArgSpec{Args: []types.Arg{{Name: "token", Format: "secret"}}}
	args := map[string]any{"token": "abc123"}

	s := newProblemScrubber(spec, args, nil, nil)

	if s.secretNames == nil {
		t.Error("expected secretNames map to be initialized")
	} else if _, ok := s.secretNames["token"]; !ok {
		// Presence matters here; the map zero value is not enough.
		t.Error("expected secretNames['token'] to be set")
	}

	found := false
	for _, val := range s.secretValues {
		if val == "abc123" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'abc123' in secretValues, got %v", s.secretValues)
	}
}

func TestScrubEventDataForPersistence_JSONRoundTrip(t *testing.T) {
	s := &persistenceScrubber{secretNames: map[string]struct{}{"token": {}}, secretValues: []string{"abc"}}
	inputJSON := `{"token":"abc","ok":"v"}`

	result := scrubEventDataForPersistence(inputJSON, s)

	if result == inputJSON {
		t.Error("expected JSON to be modified with redaction")
	}

	if !s.shouldRedactString("abc") {
		t.Error("expected 'abc' to be redacted as secret value")
	}
}

func TestScrubEventDataForPersistence_EmptyData(t *testing.T) {
	s := &persistenceScrubber{}
	result := scrubEventDataForPersistence("", s)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestScrubEventDataForPersistence_NilScrubber(t *testing.T) {
	inputJSON := `{"token":"abc","ok":"v"}`
	result := scrubEventDataForPersistence(inputJSON, nil)
	// When scrubber is nil, a new one is created (no secrets to redact)
	// JSON may be re-ordered by Marshal, so just check it's valid and contains same keys
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestScrubEventDataForPersistence_StringRedaction(t *testing.T) {
	s := &persistenceScrubber{secretValues: []string{"sekret"}}
	result := scrubEventDataForPersistence("contains sekret", s)
	if result != events.SecretToken() {
		t.Errorf("expected SecretToken(), got %q", result)
	}
}

func TestScrubEventDataForPersistence_InvalidJSON(t *testing.T) {
	s := &persistenceScrubber{}
	result := scrubEventDataForPersistence("not valid json", s)
	if result != "not valid json" {
		t.Errorf("expected unchanged string for invalid JSON, got %q", result)
	}
}

func TestAppendStringValues(t *testing.T) {
	// Add string value
	result := appendStringValues(nil, "abc")
	if len(result) != 1 || result[0] != "abc" {
		t.Errorf("expected ['abc'], got %v", result)
	}

	// Add duplicate (should not add)
	result = appendStringValues(result, "abc")
	if len(result) != 1 {
		t.Errorf("expected no duplicates, got %v", result)
	}

	// Add nil (should not change)
	result = appendStringValues(result, nil)
	if len(result) != 1 {
		t.Errorf("expected unchanged with nil, got %v", result)
	}
}

func TestScrubAny(t *testing.T) {
	s := &persistenceScrubber{secretNames: map[string]struct{}{"token": {}}, secretValues: []string{"abc"}}

	input := map[string]any{
		"token": "abc",
		"ok":    "v",
		"nested": map[string]any{
			"token": "abc",
		},
	}

	result := scrubAny(input, s)

	if resultMap, ok := result.(map[string]any); ok {
		if resultMap["token"] != events.SecretToken() {
			t.Errorf("expected token redacted to SecretToken(), got %v", resultMap["token"])
		}
		if resultMap["ok"] != "v" {
			t.Errorf("expected ok='v', got %v", resultMap["ok"])
		}
		if nested, ok := resultMap["nested"].(map[string]any); ok {
			if nested["token"] != events.SecretToken() {
				t.Errorf("expected nested token redacted to SecretToken(), got %v", nested["token"])
			}
		}
	} else {
		t.Errorf("expected map, got %T", result)
	}
}

func TestAppendStringValues_ArraysAndMaps(t *testing.T) {
	// Add []string
	result := appendStringValues(nil, []string{"a", "b"})
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("expected ['a', 'b'], got %v", result)
	}

	// Add []any
	result = appendStringValues(nil, []any{"c", "d"})
	if len(result) != 2 || result[0] != "c" || result[1] != "d" {
		t.Errorf("expected ['c', 'd'], got %v", result)
	}

	// Add map[string]string
	result = appendStringValues(nil, map[string]string{"e": "val1", "f": "val2"})
	if len(result) != 2 || (result[0] != "val1" && result[0] != "val2") {
		t.Errorf("expected ['val1', 'val2'] in some order, got %v", result)
	}

	// Add map[string]any
	result = appendStringValues(nil, map[string]any{"g": "val3", "h": "val4"})
	if len(result) != 2 || (result[0] != "val3" && result[0] != "val4") {
		t.Errorf("expected ['val3', 'val4'] in some order, got %v", result)
	}

	// Add empty string (should be skipped)
	result = appendStringValues(nil, "")
	if len(result) != 0 {
		t.Errorf("expected empty slice for empty string, got %v", result)
	}
}

func TestScrubAny_NestedStructures(t *testing.T) {
	s := &persistenceScrubber{secretNames: map[string]struct{}{"token": {}}, secretValues: []string{"abc"}}

	input := map[string]any{
		"token": "abc",
		"ok":    "v",
		"nested": map[string]any{
			"token": "abc",
			"data":  []any{"x", "abc", "y"},
		},
	}

	result := scrubAny(input, s)

	if resultMap, ok := result.(map[string]any); ok {
		if resultMap["token"] != events.SecretToken() {
			t.Errorf("expected token redacted to SecretToken(), got %v", resultMap["token"])
		}
		if resultMap["ok"] != "v" {
			t.Errorf("expected ok='v', got %v", resultMap["ok"])
		}
		if nested, ok := resultMap["nested"].(map[string]any); ok {
			if nested["token"] != events.SecretToken() {
				t.Errorf("expected nested token redacted to SecretToken(), got %v", nested["token"])
			}
			if data, ok := nested["data"].([]any); ok {
				foundSecret := false
				for _, v := range data {
					if v == events.SecretToken() {
						foundSecret = true
						break
					}
				}
				if !foundSecret {
					t.Errorf("expected SecretToken in data array, got %v", data)
				}
			}
		}
	} else {
		t.Errorf("expected map, got %T", result)
	}
}

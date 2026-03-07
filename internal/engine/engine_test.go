package engine

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/pflag"
)

func TestValidateAndBind_StringEnumRequired(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name:     "mode",
		Type:     "string",
		Required: true,
		Enum:     []string{"quick", "full"},
	}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("mode", "", "")
	_ = flags.Set("mode", "quick")

	bind, err := ValidateAndBind(flags, spec)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := bind.Values["mode"]; got != "quick" {
		t.Fatalf("expected mode=quick, got %v", got)
	}
	if bind.ArgsJSON == "" {
		t.Fatalf("expected ArgsJSON to be populated")
	}
	if env := bind.ScalarEnv["ARG_MODE"]; env != "quick" {
		t.Fatalf("expected ARG_MODE=quick, got %q", env)
	}
}

func TestValidateAndBind_ArgsJSONCanonicalAndRedacted(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name:   "alpha",
		Type:   "string",
		Format: "secret",
	}, {
		Name: "beta",
		Type: "string",
	}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("alpha", "", "")
	flags.String("beta", "", "")
	_ = flags.Set("alpha", "topsecret")
	_ = flags.Set("beta", "ok")

	bind, err := ValidateAndBind(flags, spec)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(bind.ArgsJSON), &decoded); err != nil {
		t.Fatalf("decode ArgsJSON: %v", err)
	}
	if decoded["alpha"] != "$$REDACTED$$" || decoded["beta"] != "ok" {
		t.Fatalf("unexpected ArgsJSON values: %+v", decoded)
	}
}

func TestValidateAndBind_SecretDefaultForbidden(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name:    "token",
		Type:    "string",
		Format:  "secret",
		Default: "abc",
	}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("token", "", "")

	if _, err := ValidateAndBind(flags, spec); err == nil {
		t.Fatalf("expected error for secret default, got nil")
	}
}

func TestValidateAndBind_ArrayItemsEnum(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name:      "tags",
		Type:      "array",
		ItemsType: "string",
		ItemsEnum: []string{"a", "b"},
	}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.StringArray("tags", nil, "")
	_ = flags.Set("tags", "a")
	_ = flags.Set("tags", "b")

	if _, err := ValidateAndBind(flags, spec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	flags2 := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags2.StringArray("tags", nil, "")
	_ = flags2.Set("tags", "a")
	_ = flags2.Set("tags", "c")

	if _, err := ValidateAndBind(flags2, spec); err == nil {
		t.Fatalf("expected error for invalid item")
	}
}

func TestValidateAndBind_ObjectRequiresKV(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name:      "meta",
		Type:      "object",
		ValueType: "string",
	}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.StringArray("meta", nil, "")
	if _, err := ValidateAndBind(flags, spec); err != nil {
		t.Fatalf("expected success with empty optional map, got %v", err)
	}

	flags2 := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags2.StringArray("meta", nil, "")
	_ = flags2.Set("meta", "invalidpair")
	if _, err := ValidateAndBind(flags2, spec); err == nil {
		t.Fatalf("expected error for invalid pair")
	}
}

func TestValidateAndBind_ReservedName(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name: "dry-run",
		Type: "boolean",
	}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("dry-run", false, "")
	if _, err := ValidateAndBind(flags, spec); err == nil {
		t.Fatalf("expected error for reserved arg name")
	}
}

func TestValidateAndBind_DefaultArrayTypeError(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name:    "tags",
		Type:    "array",
		Default: []interface{}{1},
	}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.StringArray("tags", nil, "")
	if _, err := ValidateAndBind(flags, spec); err == nil {
		t.Fatalf("expected error for non-string default array item")
	}
}

func TestValidateAndBind_BooleanDefault(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{Name: "enabled", Type: "boolean", Default: true}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("enabled", false, "")
	bind, err := ValidateAndBind(flags, spec)
	if err != nil {
		t.Fatalf("expected success with boolean default, got %v", err)
	}
	v, ok := bind.Values["enabled"].(bool)
	if !ok || v != true {
		t.Fatalf("expected enabled=true from default, got %v", v)
	}
	if env := bind.ScalarEnv["ARG_ENABLED"]; env != "true" {
		t.Fatalf("expected ARG_ENABLED=true, got %q", env)
	}
}

func TestValidateAndBind_IntegerDefault(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{Name: "count", Type: "integer", Default: int64(5)}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("count", 0, "")
	bind, err := ValidateAndBind(flags, spec)
	if err != nil {
		t.Fatalf("expected success with integer default, got %v", err)
	}
	v, ok := bind.Values["count"].(int)
	if !ok || v != 5 {
		t.Fatalf("expected count=5 from default, got %v", v)
	}
	if env := bind.ScalarEnv["ARG_COUNT"]; env != "5" {
		t.Fatalf("expected ARG_COUNT=5, got %q", env)
	}
}

func TestValidateAndBind_RequiredFlagMissing(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{Name: "required-flag", Type: "string", Required: true}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("required-flag", "", "")
	_, err := ValidateAndBind(flags, spec)
	if err == nil {
		t.Fatalf("expected error for required flag missing")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected 'required' in error message, got %v", err)
	}
}

func TestValidateAndBind_UnsupportedType(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{Name: "unsupported", Type: "custom"}}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("unsupported", "", "")
	_, err := ValidateAndBind(flags, spec)
	if err == nil {
		t.Fatalf("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "unsupported") || !strings.Contains(err.Error(), "\"custom\"") {
		t.Fatalf("expected 'unsupported' and '\"custom\"' in error message, got %v", err)
	}
}

func TestArgEnvName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "mode", "ARG_MODE"},
		{"hyphenated", "dry-run", "ARG_DRY_RUN"},
		{"multiple-hyphens", "max-retries-count", "ARG_MAX_RETRIES_COUNT"},
		{"already_underscored", "some_var", "ARG_SOME_VAR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := argEnvName(tt.input)
			if got != tt.expected {
				t.Errorf("argEnvName(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsSecret(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		secret   bool
		expected bool
	}{
		{"format_secret", "secret", false, true},
		{"flag_secret", "", true, true},
		{"both_set", "secret", true, true},
		{"neither_set", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSecret(tt.format, tt.secret)
			if got != tt.expected {
				t.Errorf("isSecret(%q, %v) = %v; want %v", tt.format, tt.secret, got, tt.expected)
			}
		})
	}
}

func TestMarshalCanonicalJSON_Nested(t *testing.T) {
	input := map[string]interface{}{
		"b": []interface{}{3, 2, 1},
		"a": map[string]interface{}{
			"y": "yes",
			"x": "no",
		},
	}
	data, err := marshalCanonicalJSON(input)
	if err != nil {
		t.Fatalf("marshalCanonicalJSON failed: %v", err)
	}
	got := string(data)
	expected := `{"a":{"x":"no","y":"yes"},"b":[3,2,1]}`
	if got != expected {
		t.Errorf("marshalCanonicalJSON output mismatch.\nGot:  %s\nWant: %s", got, expected)
	}
}

func TestMarshalCanonicalJSON_InvalidValue(t *testing.T) {
	type invalid struct{}
	input := map[string]interface{}{
		"key": make(chan int), // unmarshalable type
	}
	_, err := marshalCanonicalJSON(input)
	if err == nil {
		t.Fatalf("expected error for unsupported value type")
	}
}

func TestEncodeCanonicalJSON_Arrays(t *testing.T) {
	tests := []struct {
		name  string
		input []interface{}
		want  string
	}{
		{"integers", []interface{}{3, 1, 2}, "[3,1,2]"},
		{"strings", []interface{}{"b", "a", "c"}, `["b","a","c"]`},
		{"nested", []interface{}{[]interface{}{1}, []interface{}{2, 3}}, "[[1],[2,3]]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			if err := encodeCanonicalJSON(buf, tt.input); err != nil {
				t.Fatalf("encodeCanonicalJSON failed: %v", err)
			}
			got := buf.String()
			if got != tt.want {
				t.Errorf("encodeCanonicalJSON(%v) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

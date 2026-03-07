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

func TestBindArgs(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{
		Name:    "mode",
		Type:    "string",
		Default: "quick",
	}, {
		Name: "enabled",
		Type: "boolean",
	}}}

	args := map[string]interface{}{
		"mode":    "full",
		"enabled": true,
	}

	bind, err := BindArgs(spec, args)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := bind.Values["mode"]; got != "full" {
		t.Fatalf("expected mode=full, got %v", got)
	}
	if v, ok := bind.Values["enabled"].(bool); !ok || v != true {
		t.Fatalf("expected enabled=true, got %v", v)
	}
}

func TestBindArgs_NilArgs(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{Name: "mode", Type: "string"}}}
	bind, err := BindArgs(spec, nil)
	if err != nil {
		t.Fatalf("expected success with nil args, got %v", err)
	}
	// When args is nil and no default is provided, the flag gets zero value (empty string for string type)
	v, ok := bind.Values["mode"]
	if !ok || v != "" {
		t.Fatalf("expected mode=empty string, got %v, ok=%v", v, ok)
	}
}

func TestBindArgs_UnknownArg(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{Name: "known", Type: "string"}}}
	args := map[string]interface{}{"unknown": "value"}
	_, err := BindArgs(spec, args)
	if err == nil {
		t.Fatalf("expected error for unknown arg")
	}
}

func TestBindArgs_InvalidArgName(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{{Name: "valid", Type: "string"}}}
	args := map[string]interface{}{"invalid-name": "value"}
	_, err := BindArgs(spec, args)
	if err == nil {
		t.Fatalf("expected error for invalid arg name")
	}
}

func TestBindArgs_IntegerTypes(t *testing.T) {
	// Test integer with int64 default
	spec := types.ArgSpec{
		Args: []types.Arg{
			{Name: "count", Type: "integer", Default: int64(10)},
		},
	}
	bind, err := BindArgs(spec, nil)
	if err != nil {
		t.Fatalf("expected success with int64 default, got %v", err)
	}
	if v, ok := bind.Values["count"].(int); !ok || v != 10 {
		t.Fatalf("expected count=10 (int), got %v", v)
	}

	// Test integer with float64 default
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "limit", Type: "integer", Default: float64(20.5)},
		},
	}
	bind, err = BindArgs(spec, nil)
	if err != nil {
		t.Fatalf("expected success with float64 default, got %v", err)
	}
	if v, ok := bind.Values["limit"].(int); !ok || v != 20 {
		t.Fatalf("expected limit=20 (truncated), got %v", v)
	}
}

func TestBindArgs_ArrayTypes(t *testing.T) {
	// Test array with []interface{} default
	spec := types.ArgSpec{
		Args: []types.Arg{
			{Name: "items", Type: "array", Default: []interface{}{"a", "b"}},
		},
	}
	bind, err := BindArgs(spec, nil)
	if err != nil {
		t.Fatalf("expected success with array default, got %v", err)
	}
	if v, ok := bind.Values["items"].([]string); !ok || len(v) != 2 {
		t.Fatalf("expected items=[a b], got %v", v)
	}

	// Test array with []string default
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "tags", Type: "array", Default: []string{"x", "y"}},
		},
	}
	bind, err = BindArgs(spec, nil)
	if err != nil {
		t.Fatalf("expected success with string array default, got %v", err)
	}
	if v, ok := bind.Values["tags"].([]string); !ok || len(v) != 2 {
		t.Fatalf("expected tags=[x y], got %v", v)
	}

	// Test array with string default (comma-separated)
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "list", Type: "array", Default: "one,two,three"},
		},
	}
	bind, err = BindArgs(spec, nil)
	if err != nil {
		t.Fatalf("expected success with string default, got %v", err)
	}
	if v, ok := bind.Values["list"].([]string); !ok || len(v) != 3 {
		t.Fatalf("expected list=[one two three], got %v", v)
	}

	// Test array with provided []interface{}
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "items", Type: "array"},
		},
	}
	bind, err = BindArgs(spec, map[string]interface{}{"items": []interface{}{"p", "q"}})
	if err != nil {
		t.Fatalf("expected success with provided []interface{}, got %v", err)
	}
	if v, ok := bind.Values["items"].([]string); !ok || len(v) != 2 {
		t.Fatalf("expected items=[p q], got %v", v)
	}

	// Test array with provided []string
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "tags", Type: "array"},
		},
	}
	bind, err = BindArgs(spec, map[string]interface{}{"tags": []string{"r", "s"}})
	if err != nil {
		t.Fatalf("expected success with provided []string, got %v", err)
	}
	if v, ok := bind.Values["tags"].([]string); !ok || len(v) != 2 {
		t.Fatalf("expected tags=[r s], got %v", v)
	}

	// Test array with invalid item type (should fail)
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "items", Type: "array"},
		},
	}
	_, err = BindArgs(spec, map[string]interface{}{"items": []interface{}{"valid", 123}})
	if err == nil {
		t.Fatalf("expected error for invalid array item type")
	}
	if !strings.Contains(err.Error(), "must be strings") {
		t.Fatalf("expected 'must be strings' error, got %v", err)
	}

	// Test array with invalid value type (should fail)
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "items", Type: "array"},
		},
	}
	_, err = BindArgs(spec, map[string]interface{}{"items": 123})
	if err == nil {
		t.Fatalf("expected error for non-array value")
	}
	if !strings.Contains(err.Error(), "must be an array of strings") {
		t.Fatalf("expected 'must be an array of strings' error, got %v", err)
	}
}

func TestBindArgs_ObjectTypes(t *testing.T) {
	// Test object with valid map values
	spec := types.ArgSpec{
		Args: []types.Arg{
			{Name: "config", Type: "object"},
		},
	}
	bind, err := BindArgs(spec, map[string]interface{}{"config": map[string]interface{}{"key1": "val1", "key2": "val2"}})
	if err != nil {
		t.Fatalf("expected success with object, got %v", err)
	}
	if v, ok := bind.Values["config"].(map[string]string); !ok || len(v) != 2 {
		t.Fatalf("expected config map with 2 items, got %v", v)
	}

	// Test object with invalid value type (should fail)
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "config", Type: "object"},
		},
	}
	_, err = BindArgs(spec, map[string]interface{}{"config": map[string]interface{}{"key1": 123}})
	if err == nil {
		t.Fatalf("expected error for invalid object value type")
	}
	if !strings.Contains(err.Error(), "must be strings") {
		t.Fatalf("expected 'must be strings' error, got %v", err)
	}

	// Test object with non-map value (should fail)
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "config", Type: "object"},
		},
	}
	_, err = BindArgs(spec, map[string]interface{}{"config": 123})
	if err == nil {
		t.Fatalf("expected error for non-map value")
	}
	if !strings.Contains(err.Error(), "must be an object") {
		t.Fatalf("expected 'must be an object' error, got %v", err)
	}
}

func TestBindArgs_RequiredFlags(t *testing.T) {
	// Test required string flag missing
	spec := types.ArgSpec{
		Args: []types.Arg{
			{Name: "required_str", Type: "string", Required: true},
		},
	}
	_, err := BindArgs(spec, nil)
	if err == nil {
		t.Fatalf("expected error for required string missing")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected 'required' in error, got %v", err)
	}

	// Test required boolean flag missing with no default
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "required_bool", Type: "boolean", Required: true},
		},
	}
	_, err = BindArgs(spec, nil)
	if err == nil {
		t.Fatalf("expected error for required boolean missing")
	}

	// Test required integer flag missing with no default
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "required_int", Type: "integer", Required: true},
		},
	}
	_, err = BindArgs(spec, nil)
	if err == nil {
		t.Fatalf("expected error for required integer missing")
	}

	// Test required array flag missing
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "required_arr", Type: "array", Required: true},
		},
	}
	_, err = BindArgs(spec, nil)
	if err == nil {
		t.Fatalf("expected error for required array missing")
	}

	// Test required object flag missing
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "required_obj", Type: "object", Required: true},
		},
	}
	_, err = BindArgs(spec, nil)
	if err == nil {
		t.Fatalf("expected error for required object missing")
	}
}

func TestBindArgs_UnsupportedTypes(t *testing.T) {
	// Test unsupported arg type in spec
	spec := types.ArgSpec{
		Args: []types.Arg{
			{Name: "unsupported", Type: "unknown_type"},
		},
	}
	_, err := BindArgs(spec, nil)
	if err == nil {
		t.Fatalf("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "unsupported arg type") {
		t.Fatalf("expected 'unsupported arg type' in error, got %v", err)
	}

	// Test BindArgs with unknown argument
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "known", Type: "string"},
		},
	}
	_, err = BindArgs(spec, map[string]interface{}{"unknown": "value"})
	if err == nil {
		t.Fatalf("expected error for unknown argument")
	}
	if !strings.Contains(err.Error(), "unknown argument") {
		t.Fatalf("expected 'unknown argument' in error, got %v", err)
	}

	// Test BindArgs with empty args map
	spec = types.ArgSpec{
		Args: []types.Arg{
			{Name: "mode", Type: "string"},
		},
	}
	bind, err := BindArgs(spec, map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected success with empty args map, got %v", err)
	}
	_ = bind
}

package engine

import (
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

package types_test

import (
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/types"
)

// TestConfigEnvSlice covers EnvSlice behavior for nil/empty and populated maps.
func TestConfigEnvSlice(t *testing.T) {
	// Nil Config
	var c *types.Config
	s := c.EnvSlice()
	if s != nil {
		t.Errorf("(*Config)(nil).EnvSlice() = %v; want nil", s)
	}

	// Empty Env map
	c = &types.Config{Env: map[string]string{}}
	s = c.EnvSlice()
	if len(s) != 0 {
		t.Errorf("Config{{Env: map[]}}.EnvSlice() = %v; want empty", s)
	}

	// Populated Env
	c = &types.Config{Env: map[string]string{"A": "1", "B": "2"}}
	s = c.EnvSlice()
	if len(s) != 2 {
		t.Errorf("Config{{Env: map[A:1 B:2]}}.EnvSlice() length = %d; want 2", len(s))
	}
	// Check contents without relying on order
	got := make(map[string]bool)
	for _, v := range s {
		got[v] = true
	}
	if !got["A=1"] || !got["B=2"] {
		t.Errorf("Config{{Env: map[A:1 B:2]}}.EnvSlice() = %v; want [A=1 B=2] in some order", s)
	}
}

// TestValidateArgName covers ValidateArgName acceptance and rejection cases.
func TestValidateArgName(t *testing.T) {
	// Acceptance cases
	accept := []string{"foo", "foo-bar", "FOO", "_valid", "a1b2c3"}
	for _, name := range accept {
		if err := types.ValidateArgName(name); err != nil {
			t.Errorf("ValidateArgName(%q) = %v; want nil", name, err)
		}
	}

	// Rejection cases
	reject := []struct {
		name string
		want string
	}{
		{"", "name is required"},
		{"   ", "name is required"},
		{"has space", "name must not contain whitespace"},
		{"\thas tab", "name must not contain whitespace"},
		{"-starts-with-dash", "name must not start with '-'"},
		{"has=equals", "name must not contain '='"},
		{"help", "name is reserved"},
		{"HELP", "name is reserved"},
		{"version", "name is reserved"},
		{"profile", "name is reserved"},
		{"json", "name is reserved"},
	}

	for _, r := range reject {
		if err := types.ValidateArgName(r.name); err == nil {
			t.Errorf("ValidateArgName(%q) = nil; want error containing %q", r.name, r.want)
		} else if !strings.Contains(err.Error(), r.want) {
			t.Errorf("ValidateArgName(%q) = %v; want error containing %q", r.name, err, r.want)
		}
	}
}

package executor

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/types"
)

func TestBuildEnv_StripsParentEnv(t *testing.T) {
	os.Setenv("UNSAFE_VAR", "value")
	defer os.Unsetenv("UNSAFE_VAR")

	cfg := &types.Config{Env: map[string]string{"PATH": "/usr/bin"}}
	argEnv := map[string]string{"ARG_NAME": "alice"}

	env := buildSecureEnv(cfg, argEnv, "{}", false)

	for _, e := range env {
		if len(e) >= len("UNSAFE_VAR=") && e[:len("UNSAFE_VAR=")] == "UNSAFE_VAR=" {
			t.Fatalf("unexpected unsafe env in secure env: %s", e)
		}
	}

	foundArg := false
	for _, e := range env {
		if e == "ARG_NAME=alice" {
			foundArg = true
			break
		}
	}
	if !foundArg {
		t.Fatalf("ARG_NAME missing from env")
	}
}

func TestBuildSecureEnvAddsDefaultPath(t *testing.T) {
	t.Setenv("PATH", "/usr/local/bin")
	env := buildSecureEnv(nil, nil, "", false)
	expect := "PATH=/usr/local/bin"
	found := false
	for _, e := range env {
		if e == expect {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s in env: %v", expect, env)
	}
}

func TestBuildSecureEnvPrefersConfigPath(t *testing.T) {
	cfg := &types.Config{Env: map[string]string{"PATH": "/custom/bin"}}
	env := buildSecureEnv(cfg, nil, "", false)
	count := 0
	for _, e := range env {
		if e == "PATH=/custom/bin" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected config PATH to be preserved once, got %d in %v", count, env)
	}
}

func TestBuildSecureEnvInheritHost(t *testing.T) {
	key := "INHERITED_VAR"
	val := "present"
	prev := os.Getenv(key)
	t.Setenv(key, val)
	defer os.Setenv(key, prev)

	cfg := &types.Config{}
	env := buildSecureEnv(cfg, nil, "", true)
	found := false
	for _, e := range env {
		if e == key+"="+val {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected inherited env %s in %v", key, env)
	}
}

func TestBuildSecureEnvArgsJSONAliases(t *testing.T) {
	argsJSON := `{"name":"alice"}`
	env := buildSecureEnv(nil, nil, argsJSON, false)
	var flowdValue, flwdValue string
	for _, e := range env {
		switch e {
		case "FLOWD_ARGS_JSON=" + argsJSON:
			flowdValue = argsJSON
		case "FLWD_ARGS_JSON=" + argsJSON:
			flwdValue = argsJSON
		}
	}
	if flowdValue == "" {
		t.Fatalf("expected FLOWD_ARGS_JSON to be set in env: %v", env)
	}
	if flwdValue == "" {
		t.Fatalf("expected FLWD_ARGS_JSON alias to be set in env: %v", env)
	}
}

func TestExecutorEnvDoesNotExposeSecrets(t *testing.T) {
	secretValue := "supersecret"
	argsPayload := map[string]string{
		"token": events.SecretToken(),
		"name":  "alice",
	}
	argsBytes, err := json.Marshal(argsPayload)
	if err != nil {
		t.Fatalf("marshal args payload: %v", err)
	}
	argsJSON := string(argsBytes)

	env := buildSecureEnv(nil, map[string]string{"ARG_NAME": "alice"}, argsJSON, false)
	env = injectSecretHandles(env, ExecutorConfig{SecretHandles: map[string]string{
		"token": "/run/secrets/token",
	}})

	envMap := make(map[string]string, len(env))
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if envMap["FLOWD_ARGS_JSON"] != argsJSON {
		t.Fatalf("expected FLOWD_ARGS_JSON to be set to redacted args JSON")
	}
	if envMap["FLWD_ARGS_JSON"] != argsJSON {
		t.Fatalf("expected FLWD_ARGS_JSON alias to be set to redacted args JSON")
	}

	handlesJSON := envMap[secretHandlesEnv]
	if handlesJSON == "" {
		t.Fatalf("expected %s to be set", secretHandlesEnv)
	}
	if strings.Contains(handlesJSON, secretValue) {
		t.Fatalf("secret value leaked into %s", secretHandlesEnv)
	}

	var handles map[string]map[string]string
	if err := json.Unmarshal([]byte(handlesJSON), &handles); err != nil {
		t.Fatalf("decode %s: %v", secretHandlesEnv, err)
	}
	h, ok := handles["token"]
	if !ok {
		t.Fatalf("expected token handle in %s payload", secretHandlesEnv)
	}
	if h["type"] != "file" {
		t.Fatalf("expected handle type 'file', got %q", h["type"])
	}
	if h["path"] != "/run/secrets/token" {
		t.Fatalf("expected handle path, got %q", h["path"])
	}
	if len(h) != 2 {
		t.Fatalf("expected handle payload to contain only type/path, got %v", h)
	}

	for key, val := range envMap {
		if strings.Contains(val, secretValue) {
			t.Fatalf("secret value leaked into env var %s", key)
		}
	}
}

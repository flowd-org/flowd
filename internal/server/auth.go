package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

type authInfo struct {
	token   string
	subject string
	scopes  map[string]struct{}
}

func (a *authInfo) hasScopes(required []string) bool {
	if len(required) == 0 {
		return true
	}
	for _, s := range required {
		if _, ok := a.scopes[s]; !ok {
			return false
		}
	}
	return true
}

func (a *authInfo) scopesSlice() []string {
	out := make([]string, 0, len(a.scopes))
	for s := range a.scopes {
		out = append(out, s)
	}
	return out
}

func (a *authInfo) principal() string {
	if a == nil {
		return ""
	}
	if a.subject != "" {
		return a.subject
	}
	if a.token != "" {
		sum := sha256.Sum256([]byte(a.token))
		return "token:" + hex.EncodeToString(sum[:])
	}
	return "anonymous"
}

type authContextKey struct{}

var ctxAuthKey = &authContextKey{}

func withAuth(ctx context.Context, info *authInfo) context.Context {
	if info == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxAuthKey, info)
}

// AuthInfoFromContext extracts auth information from the request context.
func AuthInfoFromContext(ctx context.Context) (scopes []string, subject string, ok bool) {
	info, _ := ctx.Value(ctxAuthKey).(*authInfo)
	if info == nil {
		return nil, "", false
	}
	return info.scopesSlice(), info.subject, true
}

func parseAuthorization(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	if len(auth) < 7 || !strings.EqualFold(auth[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(auth[7:])
}

func parseToken(token string, secret string, allowUnsigned bool) (*authInfo, error) {
	if token == "" {
		return nil, errors.New("token empty")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return parseOpaqueToken(token)
	}
	header, err := decodeSegment(parts[0])
	if err != nil {
		return nil, err
	}
	_, _ = header, err
	payloadBytes, err := decodeSegment(parts[1])
	if err != nil {
		return nil, err
	}
	sig, err := decodeSegment(parts[2])
	if err != nil {
		return nil, err
	}

	if secret != "" {
		expected := computeHMAC(parts[0]+"."+parts[1], secret)
		if !hmac.Equal(sig, expected) {
			return nil, errors.New("invalid signature")
		}
	} else if !allowUnsigned {
		return nil, errors.New("unsigned token rejected")
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}
	subject, _ := claims["sub"].(string)
	scopes := extractScopes(claims)
	return &authInfo{
		token:   token,
		subject: subject,
		scopes:  scopes,
	}, nil
}

func parseOpaqueToken(token string) (*authInfo, error) {
	fields := strings.FieldsFunc(token, func(r rune) bool {
		return r == ',' || r == ' '
	})
	if len(fields) == 0 {
		return nil, errors.New("token missing scopes")
	}
	scopes := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if f == "" {
			continue
		}
		scopes[f] = struct{}{}
	}
	return &authInfo{token: token, scopes: scopes}, nil
}

func decodeSegment(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(seg)
}

func computeHMAC(data string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func extractScopes(claims map[string]any) map[string]struct{} {
	set := make(map[string]struct{})
	if raw, ok := claims["scope"].(string); ok {
		for _, s := range strings.Fields(raw) {
			set[s] = struct{}{}
		}
	}
	if rawArr, ok := claims["scopes"].([]any); ok {
		for _, v := range rawArr {
			if s, ok := v.(string); ok {
				set[s] = struct{}{}
			}
		}
	}
	return set
}

func defaultDevAuth() *authInfo {
	return &authInfo{
		token:   "dev",
		subject: "dev",
		scopes: map[string]struct{}{
			"jobs:read":     {},
			"runs:read":     {},
			"runs:write":    {},
			"events:read":   {},
			"sources:read":  {},
			"sources:write": {},
			"ruley:read":    {},
			"ruley:write":   {},
		},
	}
}

func resolveAuthInfo(r *http.Request, cfg Config) (*authInfo, error) {
	token := parseAuthorization(r)
	secret := os.Getenv("FLWD_JWT_SECRET")
	if token == "" {
		if cfg.Dev {
			return defaultDevAuth(), nil
		}
		return nil, errors.New("missing token")
	}
	info, err := parseToken(token, secret, cfg.Dev)
	if err != nil && cfg.Dev {
		// In dev mode fall back to default scopes on parse failure.
		return defaultDevAuth(), nil
	}
	return info, err
}

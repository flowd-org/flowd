package requestctx

import (
	"context"
	"log/slog"

	"github.com/flowd-org/flowd/internal/observability/logctx"
)

type profileKey struct{}
type metadataKey struct{}
type principalKey struct{}
type tenantKey struct{}

var (
	ctxProfileKey   = &profileKey{}
	ctxMetadataKey  = &metadataKey{}
	ctxPrincipalKey = &principalKey{}
	ctxTenantKey    = &tenantKey{}
)

// Metadata stores auxiliary request attributes for structured logging.
type Metadata struct {
	Runtime   string
	Route     string
	RequestID string
}

// WithLogger stores the request-scoped logger in the context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return logctx.WithLogger(ctx, logger)
}

// Logger extracts the request-scoped logger from context, if present.
func Logger(ctx context.Context) *slog.Logger {
	return logctx.Logger(ctx)
}

// WithScrubbedLogger wraps the request logger with a scrubber to redact sensitive data.
func WithScrubbedLogger(ctx context.Context, scrub func(string) string) context.Context {
	if scrub == nil {
		return ctx
	}
	logger := logctx.Logger(ctx)
	if logger == nil {
		return ctx
	}
	wrapped := logctx.WrapLogger(logger, scrub)
	if wrapped == logger {
		return ctx
	}
	return logctx.WithLogger(ctx, wrapped)
}

// WithEffectiveProfile annotates the context with the effective security profile.
func WithEffectiveProfile(ctx context.Context, profile string) context.Context {
	if profile == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxProfileKey, profile)
}

// EffectiveProfile returns the effective security profile stored in context, if any.
func EffectiveProfile(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	profile, _ := ctx.Value(ctxProfileKey).(string)
	if profile == "" {
		return "", false
	}
	return profile, true
}

// WithMetadata stores request metadata in context, overwriting any existing value.
func WithMetadata(ctx context.Context, meta *Metadata) context.Context {
	if meta == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxMetadataKey, meta)
}

// Metadata retrieves the metadata pointer stored on the context, if present.
func MetadataFromContext(ctx context.Context) *Metadata {
	if ctx == nil {
		return nil
	}
	meta, _ := ctx.Value(ctxMetadataKey).(*Metadata)
	return meta
}

// WithRuntime annotates metadata with the resolved runtime value.
func WithRuntime(ctx context.Context, runtime string) context.Context {
	if runtime == "" {
		return ctx
	}
	meta := MetadataFromContext(ctx)
	if meta == nil {
		meta = &Metadata{}
		ctx = context.WithValue(ctx, ctxMetadataKey, meta)
	}
	meta.Runtime = runtime
	return ctx
}

// WithRequestID annotates metadata with the request/correlation identifier.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	meta := MetadataFromContext(ctx)
	if meta == nil {
		meta = &Metadata{}
		ctx = context.WithValue(ctx, ctxMetadataKey, meta)
	}
	meta.RequestID = requestID
	return ctx
}

// RequestID extracts the request/correlation identifier from metadata, if any.
func RequestID(ctx context.Context) (string, bool) {
	meta := MetadataFromContext(ctx)
	if meta == nil || meta.RequestID == "" {
		return "", false
	}
	return meta.RequestID, true
}

// Runtime extracts the runtime value recorded in metadata, if any.
func Runtime(ctx context.Context) (string, bool) {
	meta := MetadataFromContext(ctx)
	if meta == nil || meta.Runtime == "" {
		return "", false
	}
	return meta.Runtime, true
}

// WithRoute annotates metadata with the templated route string.
func WithRoute(ctx context.Context, route string) context.Context {
	if route == "" {
		return ctx
	}
	meta := MetadataFromContext(ctx)
	if meta == nil {
		meta = &Metadata{}
		ctx = context.WithValue(ctx, ctxMetadataKey, meta)
	}
	meta.Route = route
	return ctx
}

// Route extracts the templated route string stored on the context, if any.
func Route(ctx context.Context) (string, bool) {
	meta := MetadataFromContext(ctx)
	if meta == nil || meta.Route == "" {
		return "", false
	}
	return meta.Route, true
}

// WithPrincipal stores the authenticated principal identifier on the context.
func WithPrincipal(ctx context.Context, principal string) context.Context {
	if principal == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxPrincipalKey, principal)
}

// Principal retrieves the authenticated principal identifier from context.
func Principal(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	principal, _ := ctx.Value(ctxPrincipalKey).(string)
	if principal == "" {
		return "", false
	}
	return principal, true
}

// WithTenant stores the authenticated tenant claim on the context.
func WithTenant(ctx context.Context, tenant string) context.Context {
	if tenant == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxTenantKey, tenant)
}

// Tenant retrieves the authenticated tenant claim from context.
func Tenant(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	tenant, _ := ctx.Value(ctxTenantKey).(string)
	if tenant == "" {
		return "", false
	}
	return tenant, true
}

// LogPolicyDecision emits a structured policy decision log using the request-scoped logger.
func LogPolicyDecision(ctx context.Context, subject, decision, code, reason string) {
	logger := Logger(ctx)
	if logger == nil {
		return
	}
	profile, _ := EffectiveProfile(ctx)
	attrs := []any{
		slog.String("subject", subject),
		slog.String("decision", decision),
	}
	if profile != "" {
		attrs = append(attrs, slog.String("profile.effective", profile))
	}
	if code != "" {
		attrs = append(attrs, slog.String("code", code))
	}
	if reason != "" {
		attrs = append(attrs, slog.String("reason", reason))
	}

	switch decision {
	case "denied", "warn", "warning":
		logger.Warn("policy_decision", attrs...)
	default:
		logger.Info("policy_decision", attrs...)
	}
}

package requestctx

import (
	"context"
	"log/slog"
)

type loggerKey struct{}
type profileKey struct{}
type metadataKey struct{}
type principalKey struct{}

var (
	ctxLoggerKey    = &loggerKey{}
	ctxProfileKey   = &profileKey{}
	ctxMetadataKey  = &metadataKey{}
	ctxPrincipalKey = &principalKey{}
)

// Metadata stores auxiliary request attributes for structured logging.
type Metadata struct {
	Runtime string
	Route   string
}

// WithLogger stores the request-scoped logger in the context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxLoggerKey, logger)
}

// Logger extracts the request-scoped logger from context, if present.
func Logger(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return nil
	}
	logger, _ := ctx.Value(ctxLoggerKey).(*slog.Logger)
	return logger
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

// SPDX-License-Identifier: AGPL-3.0-or-later

package logctx

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

var ctxLoggerKey = &loggerKey{}

// WithLogger stores the logger in the context for downstream consumers.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if ctx == nil || logger == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxLoggerKey, logger)
}

// Logger extracts a logger from context, if present.
func Logger(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return nil
	}
	logger, _ := ctx.Value(ctxLoggerKey).(*slog.Logger)
	return logger
}

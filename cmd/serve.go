// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/flowd-org/flowd/internal/server"
	"github.com/spf13/cobra"
)

// NewServeCmd creates the :serve command that bootstraps the HTTP server runtime.
func NewServeCmd() *cobra.Command {
	var (
		bindAddr       string
		logMode        string
		devMode        bool
		profile        string
		metricsEnabled bool
		aliasesPublic  bool
		extensionFlags []string
	)

	cmd := &cobra.Command{
		Use:   ":serve",
		Short: "Start Flowd in API serve mode (REST + SSE)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := server.Config{
				Bind:              bindAddr,
				Dev:               devMode,
				Log:               logMode,
				StdOut:            os.Stdout,
				StdErr:            os.Stderr,
				MetricsEnabled:    metricsEnabled,
				MetricsConfigured: true,
			}

			// Resolve profile precedence for serve: flag > env > default
			if profile == "" {
				if env := os.Getenv("FLWD_PROFILE"); env != "" {
					profile = env
				}
			}
			cfg.Profile = strings.ToLower(profile)
			cfg.AliasesPublic = resolveAliasesPublic(aliasesPublic, cmd)
			cfg.Extensions = resolveExtensions(extensionFlags, cmd)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			if err := server.Run(ctx, cfg); err != nil {
				if ctx.Err() != nil {
					// Shutdown initiated; surface as exit 0 after graceful stop.
					return nil
				}
				return fmt.Errorf("serve: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&bindAddr, "bind", "127.0.0.1:8080", "Address for HTTP server to listen on")
	cmd.Flags().BoolVar(&devMode, "dev", false, "Enable development defaults (relaxed auth, CORS)")
	cmd.Flags().StringVar(&logMode, "log", "text", "Log output format (text|json)")
	cmd.Flags().StringVar(&profile, "profile", "", "Security profile (secure|permissive|disabled); overrides FLWD_PROFILE")
	cmd.Flags().BoolVar(&metricsEnabled, "metrics", true, "Expose Prometheus /metrics endpoint")
	cmd.Flags().BoolVar(&aliasesPublic, "aliases-public", false, "Expose alias names in API responses (overrides FLWD_ALIASES_PUBLIC)")
	cmd.Flags().StringSliceVar(&extensionFlags, "extension", nil, "Enable optional extension (repeatable)")

	return cmd
}

func resolveAliasesPublic(flagValue bool, cmd *cobra.Command) bool {
	if cmd.Flags().Changed("aliases-public") {
		return flagValue
	}
	if env := os.Getenv("FLWD_ALIASES_PUBLIC"); env != "" {
		switch strings.ToLower(strings.TrimSpace(env)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return false
}

func resolveExtensions(flags []string, cmd *cobra.Command) map[string]bool {
	enabled := map[string]bool{}
	values := append([]string{}, flags...)
	if !cmd.Flags().Changed("extension") {
		if env := os.Getenv("FLWD_EXTENSIONS"); env != "" {
			envParts := strings.Split(env, ",")
			values = append(values, envParts...)
		}
	}
	for _, val := range values {
		name := strings.ToLower(strings.TrimSpace(val))
		if name == "" {
			continue
		}
		enabled[name] = true
	}
	return enabled
}

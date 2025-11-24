// SPDX-License-Identifier: AGPL-3.0-or-later
package server

import (
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/types"
)

const (
	defaultBindAddress     = "127.0.0.1:8080"
	defaultLogMode         = "text"
	defaultScriptsRoot     = "scripts"
	defaultShutdownTimeout = 15 * time.Second
	defaultRuleYLimitBytes = 32 << 20
)

// Config carries serve-mode runtime settings derived from CLI flags and env vars.
type Config struct {
	Bind                        string
	Dev                         bool
	Log                         string
	Profile                     string
	AliasesPublic               bool
	Verifier                    verify.ImageVerifier
	PolicyVerifier              verify.BundleVerifier
	ScriptsRoot                 string
	Sources                     SourcesConfig
	StdOut                      io.Writer
	StdErr                      io.Writer
	ShutdownTimeout             time.Duration
	ContainerRuntime            container.Runtime
	RuntimeDetector             RuntimeDetector
	MetricsEnabled              bool
	MetricsConfigured           bool
	MetricsAllowUnauthenticated bool
	DataDir                     string
	CoreDBOptions               coredb.Options
	CoreDB                      *coredb.DB
	RuleY                       types.RuleYConfig
	Extensions                  map[string]bool
}

// RuntimeDetector resolves the available container runtime binary.
type RuntimeDetector func() (container.Runtime, error)

// SourcesConfig carries allow-list settings for the Sources API.
type SourcesConfig struct {
	AllowLocalRoots []string
	AllowGitHosts   []string
	CheckoutDir     string
}

// normalize applies defaults when values are not supplied.
func (c Config) normalize() Config {
	if c.Bind == "" {
		c.Bind = defaultBindAddress
	}
	if c.Log == "" {
		c.Log = defaultLogMode
	}
	if c.ScriptsRoot == "" {
		c.ScriptsRoot = defaultScriptsRoot
	}
	if c.Profile == "" {
		c.Profile = "secure"
	}
	if len(c.Sources.AllowLocalRoots) == 0 {
		c.Sources.AllowLocalRoots = []string{c.ScriptsRoot}
	}
	if c.Sources.CheckoutDir == "" {
		c.Sources.CheckoutDir = paths.SourcesDir()
	}
	if c.DataDir == "" {
		c.DataDir = paths.DataDir()
	}
	if c.CoreDBOptions.DataDir == "" {
		c.CoreDBOptions.DataDir = c.DataDir
	}
	if c.StdOut == nil {
		c.StdOut = os.Stdout
	}
	if c.StdErr == nil {
		c.StdErr = os.Stderr
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = defaultShutdownTimeout
	}
	if c.RuntimeDetector == nil {
		c.RuntimeDetector = func() (container.Runtime, error) {
			return container.DetectRuntime(nil)
		}
	}
	if !c.MetricsConfigured {
		c.MetricsEnabled = true
	}
	if c.MetricsEnabled {
		c.MetricsAllowUnauthenticated = isLoopbackAddress(c.Bind)
	} else {
		c.MetricsAllowUnauthenticated = false
	}
	if len(c.RuleY.Allowlist) == 0 {
		c.RuleY.Allowlist = map[string]types.RuleYNamespaceConfig{
			"core_triggers":         {LimitBytes: defaultRuleYLimitBytes},
			"core_invocation_state": {LimitBytes: defaultRuleYLimitBytes},
		}
	} else {
		for ns, nsCfg := range c.RuleY.Allowlist {
			if nsCfg.LimitBytes <= 0 {
				nsCfg.LimitBytes = defaultRuleYLimitBytes
				c.RuleY.Allowlist[ns] = nsCfg
			}
		}
	}
	if len(c.Extensions) == 0 {
		c.Extensions = map[string]bool{}
	} else {
		normalized := make(map[string]bool, len(c.Extensions))
		for k, enabled := range c.Extensions {
			if !enabled {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(k))
			if key != "" {
				normalized[key] = true
			}
		}
		c.Extensions = normalized
	}
	return c
}

// ExtensionEnabled reports whether the supplied extension flag is enabled.
func (c Config) ExtensionEnabled(name string) bool {
	if len(c.Extensions) == 0 {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return false
	}
	return c.Extensions[key]
}

func isLoopbackAddress(bind string) bool {
	host := bind
	if strings.Contains(bind, ":") {
		parsedHost, _, err := net.SplitHostPort(bind)
		if err == nil {
			host = parsedHost
		}
	}
	if host == "" {
		host = "0.0.0.0"
	}
	if host == "*" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

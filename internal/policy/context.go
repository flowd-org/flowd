// SPDX-License-Identifier: AGPL-3.0-or-later
package policy

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// VerifyMode represents the policy mode for container image signature verification.
type VerifyMode string

const (
	VerifyModeRequired   VerifyMode = "required"
	VerifyModePermissive VerifyMode = "permissive"
	VerifyModeDisabled   VerifyMode = "disabled"
)

// Context encapsulates the loaded policy bundle and derived helpers used by the server.
type Context struct {
	bundle          *Bundle
	containerLimits *ContainerLimits
}

// ContainerLimits captures parsed resource ceilings derived from the bundle.
type ContainerLimits struct {
	CPUMillicores *int
	MemoryBytes   *int64
}

// NewContext wraps the supplied bundle. A nil bundle is valid and produces defaults.
func NewContext(bundle *Bundle) (*Context, error) {
	ctx := &Context{}
	if bundle == nil {
		return ctx, nil
	}
	ctx.bundle = bundle

	if bundle.Ceilings != nil {
		limits, err := parseContainerCeilings(bundle.Ceilings)
		if err != nil {
			return nil, err
		}
		ctx.containerLimits = limits
	}
	return ctx, nil
}

// Bundle returns the backing bundle (may be nil).
func (c *Context) Bundle() *Bundle {
	if c == nil {
		return nil
	}
	return c.bundle
}

// VerifyModeForProfile returns the effective signature verification mode for the
// provided security profile, applying cookbook defaults when the bundle does not
// override the value.
func (c *Context) VerifyModeForProfile(profile string) (VerifyMode, error) {
	var explicit *string
	if c != nil && c.bundle != nil {
		explicit = c.bundle.VerifySignatures
	}
	if explicit != nil {
		mode, ok := NormalizeVerifySignatures(*explicit)
		if !ok {
			return "", fmt.Errorf("invalid verify_signatures: %q", *explicit)
		}
		return VerifyMode(mode), nil
	}

	switch lower(profile) {
	case "secure":
		return VerifyModeRequired, nil
	case "permissive":
		return VerifyModePermissive, nil
	case "disabled":
		return VerifyModeDisabled, nil
	default:
		return VerifyModeRequired, fmt.Errorf("unknown profile %q", profile)
	}
}

// AllowedRegistries returns the allow-list of registries declared in the bundle.
func (c *Context) AllowedRegistries() []string {
	if c == nil || c.bundle == nil {
		return nil
	}
	return c.bundle.AllowedRegistries
}

// Ceilings returns the resource ceilings declared in the bundle (may be nil).
func (c *Context) Ceilings() *Ceilings {
	if c == nil || c.bundle == nil {
		return nil
	}
	return c.bundle.Ceilings
}

// Overrides returns the override rules declared in the bundle (may be nil).
func (c *Context) Overrides() *Overrides {
	if c == nil || c.bundle == nil {
		return nil
	}
	return c.bundle.Overrides
}

// ContainerCeilings returns parsed container resource ceilings (may be nil if unspecified).
func (c *Context) ContainerCeilings() *ContainerLimits {
	if c == nil {
		return nil
	}
	return c.containerLimits
}

func parseContainerCeilings(src *Ceilings) (*ContainerLimits, error) {
	if src == nil {
		return nil, nil
	}
	limits := &ContainerLimits{}
	if v := strings.TrimSpace(src.CPU); v != "" {
		val, err := ParseCPUMillicores(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ceilings.cpu: %w", err)
		}
		limits.CPUMillicores = &val
	}
	if v := strings.TrimSpace(src.Memory); v != "" {
		val, err := ParseMemoryBytes(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ceilings.memory: %w", err)
		}
		limits.MemoryBytes = &val
	}
	if limits.CPUMillicores == nil && limits.MemoryBytes == nil {
		return nil, nil
	}
	return limits, nil
}

// ParseCPUMillicores parses CPU values expressed in cores or millicores.
func ParseCPUMillicores(value string) (int, error) {
	lowerVal := strings.ToLower(strings.TrimSpace(value))
	if lowerVal == "" {
		return 0, fmt.Errorf("empty cpu value")
	}
	if strings.HasSuffix(lowerVal, "m") {
		num := strings.TrimSpace(strings.TrimSuffix(lowerVal, "m"))
		if num == "" {
			return 0, fmt.Errorf("invalid millicores value %q", value)
		}
		mc, err := strconv.Atoi(num)
		if err != nil {
			return 0, fmt.Errorf("parse millicores %q: %w", value, err)
		}
		if mc < 0 {
			return 0, fmt.Errorf("cpu value must be positive")
		}
		return mc, nil
	}
	cores, err := strconv.ParseFloat(lowerVal, 64)
	if err != nil {
		return 0, fmt.Errorf("parse cores %q: %w", value, err)
	}
	if cores < 0 {
		return 0, fmt.Errorf("cpu value must be positive")
	}
	mc := int(math.Round(cores * 1000))
	return mc, nil
}

// ParseMemoryBytes parses memory values using binary units (Ki/Mi/Gi).
func ParseMemoryBytes(value string) (int64, error) {
	lowerVal := strings.ToLower(strings.TrimSpace(value))
	if lowerVal == "" {
		return 0, fmt.Errorf("empty memory value")
	}
	type unit struct {
		suffix string
		scale  float64
	}
	units := []unit{
		{"gib", 1024 * 1024 * 1024},
		{"gi", 1024 * 1024 * 1024},
		{"g", 1024 * 1024 * 1024},
		{"mib", 1024 * 1024},
		{"mi", 1024 * 1024},
		{"m", 1024 * 1024},
		{"kib", 1024},
		{"ki", 1024},
		{"k", 1024},
	}
	for _, u := range units {
		if strings.HasSuffix(lowerVal, u.suffix) {
			num := strings.TrimSpace(strings.TrimSuffix(lowerVal, u.suffix))
			if num == "" {
				return 0, fmt.Errorf("invalid memory value %q", value)
			}
			amt, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("parse memory %q: %w", value, err)
			}
			if amt < 0 {
				return 0, fmt.Errorf("memory value must be positive")
			}
			return int64(math.Round(amt * u.scale)), nil
		}
	}
	amt, err := strconv.ParseFloat(lowerVal, 64)
	if err != nil {
		return 0, fmt.Errorf("parse memory %q: %w", value, err)
	}
	if amt < 0 {
		return 0, fmt.Errorf("memory value must be positive")
	}
	return int64(math.Round(amt)), nil
}

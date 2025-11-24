// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/pflag"
)

type Binding struct {
	Values       map[string]interface{}
	ArgsJSON     string
	ScalarEnv    map[string]string // ARG_<UPPER> for scalar types only
	SecretNames  map[string]struct{}
	SecretValues []string
}

type ArgError struct {
	Arg string
	Msg string
}

func (e *ArgError) Error() string { return fmt.Sprintf("arg %s: %s", e.Arg, e.Msg) }

// ValidateAndBind validates flags against ArgSpec and returns a binding map + JSON
func ValidateAndBind(flags *pflag.FlagSet, spec types.ArgSpec) (*Binding, error) {
	vals := make(map[string]interface{})
	scalars := make(map[string]string)
	secretNames := make(map[string]struct{})
	var secretValues []string

	for _, a := range spec.Args {
		name := a.Name
		provided := flags.Changed(name)

		// secret defaults are forbidden
		if (a.Format == "secret" || a.Secret) && a.Default != nil {
			return nil, &ArgError{Arg: name, Msg: "default forbidden for secret"}
		}

		switch a.Type {
		case "string":
			var v string
			if provided {
				v, _ = flags.GetString(name)
			} else if a.Default != nil {
				v, _ = a.Default.(string)
			}
			if a.Required && v == "" {
				return nil, &ArgError{Arg: name, Msg: "required"}
			}
			if len(a.Enum) > 0 && v != "" {
				if !contains(a.Enum, v) {
					return nil, &ArgError{Arg: name, Msg: fmt.Sprintf("value %q not in enum", v)}
				}
			}
			vals[name] = v
			if isSecret(a.Format, a.Secret) {
				secretNames[name] = struct{}{}
				if v != "" {
					secretValues = append(secretValues, v)
				}
			} else {
				scalars[argEnvName(name)] = v
			}

		case "boolean":
			var v bool
			if provided {
				v, _ = flags.GetBool(name)
			} else if a.Default != nil {
				if b, ok := a.Default.(bool); ok {
					v = b
				}
			}
			if a.Required && !provided && a.Default == nil {
				return nil, &ArgError{Arg: name, Msg: "required"}
			}
			vals[name] = v
			scalars[argEnvName(name)] = fmt.Sprintf("%t", v)

		case "integer":
			var v int
			if provided {
				v, _ = flags.GetInt(name)
			} else if a.Default != nil {
				switch dv := a.Default.(type) {
				case int:
					v = dv
				case int64:
					v = int(dv)
				case float64:
					v = int(dv)
				}
			}
			if a.Required && !provided && a.Default == nil {
				return nil, &ArgError{Arg: name, Msg: "required"}
			}
			vals[name] = v
			scalars[argEnvName(name)] = fmt.Sprintf("%d", v)

		case "array":
			// Phase 1: array<string>
			arr, _ := flags.GetStringArray(name)
			// default fallback if not provided and default present
			if len(arr) == 0 && a.Default != nil {
				// allow default to be []string or comma-separated string
				switch dv := a.Default.(type) {
				case []interface{}:
					for _, it := range dv {
						if s, ok := it.(string); ok {
							arr = append(arr, s)
						}
					}
				case []string:
					arr = append(arr, dv...)
				case string:
					if dv != "" {
						arr = strings.Split(dv, ",")
					}
				}
			}
			if a.Required && len(arr) == 0 {
				return nil, &ArgError{Arg: name, Msg: "required"}
			}
			if a.ItemsType != "" && a.ItemsType != "string" {
				return nil, &ArgError{Arg: name, Msg: "items_type not supported in Phase 1"}
			}
			if len(a.ItemsEnum) > 0 {
				for _, it := range arr {
					if !contains(a.ItemsEnum, it) {
						return nil, &ArgError{Arg: name, Msg: fmt.Sprintf("item %q not in items_enum", it)}
					}
				}
			}
			vals[name] = arr

		case "object":
			// Accept repeated --name k=v (string values only in Phase 1)
			pairs, _ := flags.GetStringArray(name)
			m := map[string]string{}
			for _, p := range pairs {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) != 2 {
					return nil, &ArgError{Arg: name, Msg: fmt.Sprintf("invalid pair %q, expected k=v", p)}
				}
				k := strings.TrimSpace(kv[0])
				v := kv[1]
				if k == "" {
					return nil, &ArgError{Arg: name, Msg: "empty key"}
				}
				m[k] = v
			}
			if a.Required && len(m) == 0 {
				return nil, &ArgError{Arg: name, Msg: "required"}
			}
			if a.ValueType != "" && a.ValueType != "string" {
				return nil, &ArgError{Arg: name, Msg: "value_type not supported in Phase 1"}
			}
			vals[name] = m

		default:
			return nil, &ArgError{Arg: name, Msg: fmt.Sprintf("unsupported type %q", a.Type)}
		}
	}

	argsJSON, err := json.Marshal(vals)
	if err != nil {
		return nil, fmt.Errorf("encode args json: %w", err)
	}

	b := &Binding{Values: vals, ArgsJSON: string(argsJSON), ScalarEnv: scalars}
	if len(secretNames) > 0 {
		b.SecretNames = secretNames
	}
	if len(secretValues) > 0 {
		b.SecretValues = secretValues
	}
	return b, nil
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func argEnvName(name string) string {
	up := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return "ARG_" + up
}

func isSecret(format string, secret bool) bool {
	if secret {
		return true
	}
	return format == "secret"
}

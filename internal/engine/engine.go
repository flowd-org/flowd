// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/secrets"
	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/pflag"
)

type Binding struct {
	Values        map[string]interface{}
	ArgsJSON      string
	ScalarEnv     map[string]string // ARG_<UPPER> for scalar types only
	SecretNames   map[string]struct{}
	SecretValues  []string
	SecretBuffers map[string]*secrets.Buffer
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
	secretBuffers := map[string]*secrets.Buffer{}

	for _, a := range spec.Args {
		if err := types.ValidateArgName(a.Name); err != nil {
			return nil, &ArgError{Arg: a.Name, Msg: err.Error()}
		}
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
					secretBuffers[name] = secrets.NewBufferFromString(v)
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
			if !isSecret(a.Format, a.Secret) {
				scalars[argEnvName(name)] = fmt.Sprintf("%t", v)
			}

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
			if !isSecret(a.Format, a.Secret) {
				scalars[argEnvName(name)] = fmt.Sprintf("%d", v)
			}

		case "array":
			arr, _ := flags.GetStringArray(name)
			// default fallback if not provided and default present
			if len(arr) == 0 && a.Default != nil {
				switch dv := a.Default.(type) {
				case []interface{}:
					for _, it := range dv {
						s, ok := it.(string)
						if !ok {
							return nil, &ArgError{Arg: name, Msg: "default array items must be strings"}
						}
						arr = append(arr, s)
					}
				case []string:
					arr = append(arr, dv...)
				case string:
					if dv != "" {
						arr = append(arr, strings.Split(dv, ",")...)
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

	argsJSON, err := marshalCanonicalJSON(events.RedactSecrets(vals, secretNames))
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
	if len(secretBuffers) > 0 {
		b.SecretBuffers = secretBuffers
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

func marshalCanonicalJSON(vals map[string]interface{}) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := encodeCanonicalJSON(buf, vals); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeCanonicalJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			if err := encodeCanonicalJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case map[string]string:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			writeJSONString(buf, t[k])
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeCanonicalJSON(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case []string:
		buf.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, elem)
		}
		buf.WriteByte(']')
	case string:
		writeJSONString(buf, t)
	case json.Number:
		buf.WriteString(t.String())
	case float64:
		buf.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
	case int:
		buf.WriteString(strconv.Itoa(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func writeJSONString(buf *bytes.Buffer, s string) {
	b, _ := json.Marshal(s)
	buf.Write(b)
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

// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"io"
	"strconv"

	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/pflag"
)

// BindArgs validates args against spec and returns an engine Binding.
// This mirrors the HTTP planning path, but lives in engine so orchestration can be shared.
func BindArgs(spec types.ArgSpec, args map[string]interface{}) (*Binding, error) {
	if args == nil {
		args = map[string]interface{}{}
	}

	fs := pflag.NewFlagSet("engine-bind", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := attachSpecFlagsForBind(fs, spec); err != nil {
		return nil, err
	}

	for name := range args {
		if err := types.ValidateArgName(name); err != nil {
			return nil, &ArgError{Arg: name, Msg: err.Error()}
		}
		if !hasArgInSpec(spec, name) {
			return nil, &ArgError{Arg: name, Msg: "unknown argument"}
		}
	}

	for _, arg := range spec.Args {
		if err := types.ValidateArgName(arg.Name); err != nil {
			return nil, &ArgError{Arg: arg.Name, Msg: err.Error()}
		}
		val, ok := args[arg.Name]
		if !ok {
			continue
		}
		if err := setFlagFromValueForBind(fs, arg, val); err != nil {
			return nil, err
		}
	}

	return ValidateAndBind(fs, spec)
}

func attachSpecFlagsForBind(fs *pflag.FlagSet, spec types.ArgSpec) error {
	for _, a := range spec.Args {
		if err := types.ValidateArgName(a.Name); err != nil {
			return &ArgError{Arg: a.Name, Msg: err.Error()}
		}
		switch a.Type {
		case "string":
			def, _ := a.Default.(string)
			fs.String(a.Name, def, "")
		case "boolean":
			def, _ := a.Default.(bool)
			fs.Bool(a.Name, def, "")
		case "integer":
			var defInt int
			switch v := a.Default.(type) {
			case int:
				defInt = v
			case int64:
				defInt = int(v)
			case float64:
				defInt = int(v)
			}
			fs.Int(a.Name, defInt, "")
		case "array", "object":
			fs.StringArray(a.Name, nil, "")
		default:
			return &ArgError{Arg: a.Name, Msg: "unsupported arg type"}
		}
	}
	return nil
}

func setFlagFromValueForBind(fs *pflag.FlagSet, arg types.Arg, val interface{}) error {
	switch arg.Type {
	case "string":
		s, ok := val.(string)
		if !ok {
			return &ArgError{Arg: arg.Name, Msg: "must be a string"}
		}
		return fs.Set(arg.Name, s)
	case "boolean":
		b, ok := val.(bool)
		if !ok {
			return &ArgError{Arg: arg.Name, Msg: "must be a boolean"}
		}
		return fs.Set(arg.Name, strconv.FormatBool(b))
	case "integer":
		switch v := val.(type) {
		case float64:
			return fs.Set(arg.Name, strconv.Itoa(int(v)))
		case int:
			return fs.Set(arg.Name, strconv.Itoa(v))
		case int64:
			return fs.Set(arg.Name, strconv.Itoa(int(v)))
		default:
			return &ArgError{Arg: arg.Name, Msg: "must be an integer"}
		}
	case "array":
		switch arr := val.(type) {
		case []interface{}:
			for _, item := range arr {
				s, ok := item.(string)
				if !ok {
					return &ArgError{Arg: arg.Name, Msg: "array items must be strings"}
				}
				if err := fs.Set(arg.Name, s); err != nil {
					return err
				}
			}
			return nil
		case []string:
			for _, item := range arr {
				if err := fs.Set(arg.Name, item); err != nil {
					return err
				}
			}
			return nil
		default:
			return &ArgError{Arg: arg.Name, Msg: "must be an array of strings"}
		}
	case "object":
		mp, ok := val.(map[string]interface{})
		if !ok {
			return &ArgError{Arg: arg.Name, Msg: "must be an object"}
		}
		for k, v := range mp {
			str, ok := v.(string)
			if !ok {
				return &ArgError{Arg: arg.Name, Msg: "object values must be strings"}
			}
			if err := fs.Set(arg.Name, k+"="+str); err != nil {
				return err
			}
		}
		return nil
	default:
		return &ArgError{Arg: arg.Name, Msg: "unsupported arg type"}
	}
}

func hasArgInSpec(spec types.ArgSpec, name string) bool {
	for _, arg := range spec.Args {
		if arg.Name == name {
			return true
		}
	}
	return false
}

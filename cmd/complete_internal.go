// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type completionCandidate struct {
	Insert  string `json:"insert"`
	Display string `json:"display"`
	Type    string `json:"type"`
}

func NewInternalCompleteCmd(root *cobra.Command) *cobra.Command {
	resolver := newCompletionResolver(root)

	cmd := &cobra.Command{
		Use:    "__complete <cursor-index> [argv...]",
		Short:  "Hidden command powering shell completion",
		Hidden: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("cursor index argument required")
			}
			if _, err := strconv.Atoi(args[0]); err != nil {
				return fmt.Errorf("invalid cursor index %q", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cursor, _ := strconv.Atoi(args[0])
			tokens := append([]string(nil), args[1:]...)
			candidates, err := resolver.Resolve(cursor, tokens)
			if err != nil {
				return err
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			for _, cand := range candidates {
				if err := enc.Encode(cand); err != nil {
					return err
				}
			}
			return nil
		},
	}
	return cmd
}

type completionResolver struct {
	root         *cobra.Command
	argSpecCache map[string]*types.ArgSpec
	cacheMu      sync.RWMutex
}

func newCompletionResolver(root *cobra.Command) *completionResolver {
	return &completionResolver{
		root:         root,
		argSpecCache: make(map[string]*types.ArgSpec),
	}
}

func (r *completionResolver) Resolve(cursor int, tokens []string) ([]completionCandidate, error) {
	_ = cursor
	prefix, current := splitTokens(tokens)

	contextCmd := r.resolveContext(prefix)
	if contextCmd == nil {
		contextCmd = r.root
	}

	pendingFlag := findPendingFlag(prefix)
	isJob := commandIsJob(contextCmd)

	switch {
	case pendingFlag != "":
		return r.valueCandidates(contextCmd, pendingFlag, current), nil
	case strings.HasPrefix(current, "-") || isJob:
		return r.flagCandidates(contextCmd, current), nil
	default:
		return segmentCandidates(contextCmd, current), nil
	}
}

func splitTokens(tokens []string) ([]string, string) {
	if len(tokens) == 0 {
		return nil, ""
	}
	current := tokens[len(tokens)-1]
	prefix := tokens[:len(tokens)-1]
	return prefix, current
}

func (r *completionResolver) resolveContext(prefix []string) *cobra.Command {
	if len(prefix) == 0 {
		return r.root
	}

	cmd := r.root
	for i := 0; i < len(prefix); i++ {
		token := prefix[i]
		if strings.HasPrefix(token, "-") {
			break
		}
		next := findSubcommand(cmd, token)
		if next == nil {
			break
		}
		cmd = next
	}
	return cmd
}

func findSubcommand(parent *cobra.Command, token string) *cobra.Command {
	lower := strings.ToLower(token)
	for _, child := range parent.Commands() {
		name := commandName(child)
		if name == "" {
			continue
		}
		if strings.ToLower(name) == lower {
			return child
		}
	}
	return nil
}

func commandName(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	fields := strings.Fields(cmd.Use)
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	if name == "__complete" {
		return ""
	}
	return name
}

func commandIsJob(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Annotations == nil {
		return false
	}
	if cmd.Annotations["scriptDir"] != "" {
		return true
	}
	if cmd.Annotations["isAlias"] == "true" {
		return true
	}
	return false
}

func segmentCandidates(parent *cobra.Command, current string) []completionCandidate {
	wantInternal := strings.HasPrefix(current, ":")
	lower := strings.ToLower(current)

	children := parent.Commands()
	out := make([]completionCandidate, 0, len(children))
	for _, child := range children {
		if child.Hidden {
			continue
		}
		name := commandName(child)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, ":") && !wantInternal {
			continue
		}
		if lower != "" && !strings.HasPrefix(strings.ToLower(name), lower) {
			continue
		}
		display := child.Short
		if display == "" {
			display = name
		}
		if child.Annotations != nil && child.Annotations["isAlias"] == "true" {
			target := child.Annotations["aliasTarget"]
			desc := child.Annotations["aliasDescription"]
			switch {
			case target != "" && desc != "":
				display = fmt.Sprintf("%s (alias -> %s, %s)", name, target, desc)
			case target != "":
				display = fmt.Sprintf("%s (alias -> %s)", name, target)
			}
		}
		out = append(out, completionCandidate{
			Insert:  name,
			Display: display,
			Type:    "segment",
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Insert == out[j].Insert {
			return out[i].Display < out[j].Display
		}
		return out[i].Insert < out[j].Insert
	})
	return out
}

func findPendingFlag(prefix []string) string {
	if len(prefix) == 0 {
		return ""
	}
	expectingValue := false
	for i := len(prefix) - 1; i >= 0; i-- {
		token := prefix[i]
		if strings.HasPrefix(token, "-") {
			if expectingValue {
				expectingValue = false
				continue
			}
			return token
		}
		expectingValue = true
	}
	return ""
}

func (r *completionResolver) flagCandidates(cmd *cobra.Command, current string) []completionCandidate {
	flagSet := cmd.Flags()
	if flagSet == nil {
		return nil
	}
	prefix := strings.ToLower(current)
	type flagEntry struct {
		name    string
		display string
	}
	var flags []flagEntry
	seen := make(map[string]struct{})

	flagSet.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		long := "--" + f.Name
		key := strings.ToLower(long)
		if _, ok := seen[key]; ok {
			return
		}
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return
		}
		display := f.Usage
		if display == "" {
			display = f.Name
		}
		flags = append(flags, flagEntry{name: long, display: display})
		seen[key] = struct{}{}
		if f.Shorthand != "" {
			short := "-" + f.Shorthand
			skey := strings.ToLower(short)
			if prefix == "" || strings.HasPrefix(skey, prefix) {
				if _, ok := seen[skey]; !ok {
					flags = append(flags, flagEntry{name: short, display: display})
					seen[skey] = struct{}{}
				}
			}
		}
	})

	sort.Slice(flags, func(i, j int) bool {
		if flags[i].name == flags[j].name {
			return flags[i].display < flags[j].display
		}
		return flags[i].name < flags[j].name
	})

	out := make([]completionCandidate, 0, len(flags))
	for _, entry := range flags {
		out = append(out, completionCandidate{
			Insert:  entry.name,
			Display: entry.display,
			Type:    "flag",
		})
	}
	return out
}

func (r *completionResolver) valueCandidates(cmd *cobra.Command, flagToken, current string) []completionCandidate {
	if !strings.HasPrefix(flagToken, "-") {
		return nil
	}
	name := normalizeFlagName(flagToken)
	if name == "" {
		return nil
	}

	var arg *types.Arg
	scriptDir := ""
	if cmd.Annotations != nil {
		scriptDir = cmd.Annotations["scriptDir"]
	}
	if scriptDir != "" {
		argSpec := r.lookupArgSpec(scriptDir)
		if argSpec != nil {
			for i := range argSpec.Args {
				if argSpec.Args[i].Name == name {
					arg = &argSpec.Args[i]
					break
				}
			}
		}
	}

	flagSet := cmd.Flags()
	flag := flagSet.Lookup(name)
	if flag == nil && strings.HasPrefix(flagToken, "-") && len(flagToken) == 2 {
		// short flag; attempt lookup via shorthand
		flagSet.VisitAll(func(f *pflag.Flag) {
			if f.Shorthand == flagToken[1:] {
				flag = f
			}
		})
	}
	if flag == nil {
		return nil
	}
	if flag.Value.Type() == "bool" && arg == nil {
		return []completionCandidate{
			{Insert: "true", Display: "true", Type: "value"},
			{Insert: "false", Display: "false", Type: "value"},
		}
	}

	values := deriveValueHints(arg, flag)
	if len(values) == 0 {
		return nil
	}

	prefix := strings.ToLower(current)
	out := make([]completionCandidate, 0, len(values))
	for _, v := range values {
		if prefix != "" && !strings.HasPrefix(strings.ToLower(v.insert), prefix) {
			continue
		}
		out = append(out, completionCandidate{
			Insert:  v.insert,
			Display: v.display,
			Type:    "value",
		})
	}
	return out
}

type valueHint struct {
	insert  string
	display string
}

func deriveValueHints(arg *types.Arg, flag *pflag.Flag) []valueHint {
	if arg == nil {
		if flag.Value.Type() == "bool" {
			return []valueHint{{insert: "true", display: "true"}, {insert: "false", display: "false"}}
		}
		return nil
	}

	switch arg.Type {
	case "boolean":
		return []valueHint{{insert: "true", display: "true"}, {insert: "false", display: "false"}}
	case "string", "integer":
		if len(arg.Enum) > 0 {
			return enumHints(arg.Enum)
		}
	case "array":
		if len(arg.ItemsEnum) > 0 {
			return enumHints(arg.ItemsEnum)
		}
	case "object":
		if len(arg.Enum) > 0 {
			return enumHints(arg.Enum)
		}
	}

	return nil
}

func enumHints(values []string) []valueHint {
	out := make([]valueHint, 0, len(values))
	for _, v := range values {
		out = append(out, valueHint{insert: v, display: v})
	}
	return out
}

func normalizeFlagName(token string) string {
	token = strings.TrimLeft(token, "-")
	if token == "" {
		return ""
	}
	if idx := strings.Index(token, "="); idx >= 0 {
		token = token[:idx]
	}
	return token
}

func (r *completionResolver) lookupArgSpec(scriptDir string) *types.ArgSpec {
	r.cacheMu.RLock()
	spec, ok := r.argSpecCache[scriptDir]
	r.cacheMu.RUnlock()
	if ok {
		return spec
	}

	cfg, err := configloader.LoadConfig(scriptDir)
	if err != nil {
		r.cacheMu.Lock()
		r.argSpecCache[scriptDir] = nil
		r.cacheMu.Unlock()
		return nil
	}
	r.cacheMu.Lock()
	r.argSpecCache[scriptDir] = cfg.ArgSpec
	r.cacheMu.Unlock()
	return cfg.ArgSpec
}

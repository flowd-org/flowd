// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/types"
)

type Orchestrator struct {
	jobResolver JobResolver
	loader      ConfigLoader
	runIDGen    RunIDGenerator
}

type OrchestratorDeps struct {
	JobResolver  JobResolver
	ConfigLoader ConfigLoader
	RunIDGen     RunIDGenerator
}

func NewOrchestrator(deps OrchestratorDeps) *Orchestrator {
	o := &Orchestrator{jobResolver: deps.JobResolver, loader: deps.ConfigLoader, runIDGen: deps.RunIDGen}
	if o.loader == nil {
		o.loader = defaultConfigLoader{}
	}
	if o.runIDGen == nil {
		o.runIDGen = defaultRunIDGen{}
	}
	return o
}

type StartRunResult struct {
	RunID          string
	JobID          string
	EffectiveJobID string
	ScriptDir      string
	Config         *types.Config
	Binding        *Binding
	Plan           types.Plan
}

func (o *Orchestrator) StartRun(ctx context.Context, req types.StartRunRequest) (StartRunResult, error) {
	_ = ctx
	var res StartRunResult
	if req.JobID == "" {
		return res, errors.New("job_id required")
	}

	scriptDir := req.ScriptDir
	effectiveJobID := req.JobID
	if scriptDir == "" {
		if o.jobResolver == nil {
			return res, errors.New("script_dir required when no job resolver is configured")
		}
		dir, eff, err := o.jobResolver.Resolve(ctx, req.JobID)
		if err != nil {
			return res, fmt.Errorf("resolve job: %w", err)
		}
		scriptDir = dir
		if eff != "" {
			effectiveJobID = eff
		}
	}

	cfg, err := o.loader.LoadConfig(scriptDir)
	if err != nil {
		return res, fmt.Errorf("load config: %w", err)
	}

	var spec types.ArgSpec
	if cfg != nil && cfg.ArgSpec != nil {
		spec = *cfg.ArgSpec
	}

	bind, err := BindArgs(spec, req.Args)
	if err != nil {
		return res, fmt.Errorf("bind args: %w", err)
	}

	plan := BuildPlan(effectiveJobID, cfg, &spec, bind)

	res.RunID = o.runIDGen.NewRunID()
	res.JobID = req.JobID
	res.EffectiveJobID = effectiveJobID
	res.ScriptDir = scriptDir
	res.Config = cfg
	res.Binding = bind
	res.Plan = plan
	return res, nil
}

type defaultConfigLoader struct{}

func (defaultConfigLoader) LoadConfig(scriptDir string) (*types.Config, error) {
	return configloader.LoadConfig(scriptDir)
}

type defaultRunIDGen struct{}

func (defaultRunIDGen) NewRunID() string { return events.GenerateRunID() }

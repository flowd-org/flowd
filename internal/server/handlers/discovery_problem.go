// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"errors"
	"net/http"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/server/response"
)

func discoveryProblem(err error) (*response.Problem, bool) {
	var invalid indexer.InvalidJobIDError
	if errors.As(err, &invalid) {
		prob := response.New(http.StatusConflict, "invalid job id",
			response.WithType(response.ProblemTypeJobIDInvalidSegment),
			response.WithExtension("code", response.ProblemCodeJobIDInvalidSegment),
			response.WithDetail(invalid.Error()),
			response.WithExtension("segment", invalid.Segment),
			response.WithExtension("path", invalid.Path),
			response.WithExtension("reason", invalid.Reason),
		)
		if invalid.JobDir != "" {
			prob.Ext["job_dir"] = invalid.JobDir
		}
		return &prob, true
	}
	var idErr indexer.JobIDError
	if errors.As(err, &idErr) {
		prob := response.New(http.StatusConflict, "invalid job id",
			response.WithType(response.ProblemTypeJobIDInvalidSegment),
			response.WithExtension("code", response.ProblemCodeJobIDInvalidSegment),
			response.WithDetail(idErr.Error()),
			response.WithExtension("segment", idErr.Segment),
			response.WithExtension("path", idErr.Path),
			response.WithExtension("reason", idErr.Reason),
		)
		return &prob, true
	}
	var dual *configloader.DualConfigError
	if errors.As(err, &dual) {
		prob := response.New(http.StatusConflict, "invalid job configuration",
			response.WithType(response.ProblemTypeJobConfigDualSentinel),
			response.WithExtension("code", configloader.DualConfigCode),
			response.WithDetail(dual.Error()),
			response.WithExtension("primary_path", dual.PrimaryPath),
			response.WithExtension("legacy_path", dual.LegacyPath),
		)
		return &prob, true
	}
	return nil, false
}

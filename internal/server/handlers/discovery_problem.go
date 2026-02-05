// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"errors"
	"net/http"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/server/response"
)

func discoveryProblem(err error) (*response.Problem, bool) {
	var dual *configloader.DualConfigError
	if errors.As(err, &dual) {
		prob := response.New(http.StatusConflict, "invalid job configuration",
			response.WithExtension("code", configloader.DualConfigCode),
			response.WithDetail(dual.Error()),
			response.WithExtension("primary_path", dual.PrimaryPath),
			response.WithExtension("legacy_path", dual.LegacyPath),
		)
		return &prob, true
	}
	return nil, false
}

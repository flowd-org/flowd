// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"net/http"

	"github.com/flowd-org/flowd/internal/server/response"
)

func runtimeUnavailableProblem(err error) response.Problem {
	opts := []response.Option{response.WithExtension("code", "container.runtime.unavailable")}
	if err != nil && err.Error() != "" {
		opts = append(opts, response.WithDetail(err.Error()))
	}
	return response.New(http.StatusUnprocessableEntity, "container runtime unavailable", opts...)
}

func containerNameConflictProblem(err error) response.Problem {
	opts := []response.Option{response.WithExtension("code", "container.name.conflict")}
	if err != nil && err.Error() != "" {
		opts = append(opts, response.WithDetail(err.Error()))
	}
	return response.New(http.StatusUnprocessableEntity, "container name conflict", opts...)
}

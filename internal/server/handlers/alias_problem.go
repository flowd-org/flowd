package handlers

import (
	"fmt"
	"net/http"

	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/server/response"
)

func aliasCollisionProblem(aliasName string, contenders []indexer.AliasInfo) *response.Problem {
	payload := make([]map[string]string, 0, len(contenders))
	for _, c := range contenders {
		entry := map[string]string{
			"name":        c.Name,
			"target_id":   c.TargetID,
			"target_path": c.TargetPath,
		}
		if c.Source != "" {
			entry["source"] = c.Source
		}
		payload = append(payload, entry)
	}
	detail := fmt.Sprintf("alias %q resolves to multiple contenders", aliasName)
	prob := response.New(http.StatusConflict, "alias collision",
		response.WithExtension("code", "alias.collision"),
		response.WithExtension("alias", aliasName),
		response.WithExtension("contenders", payload),
		response.WithDetail(detail))
	return &prob
}

func aliasValidationProblem(aliasName string, validation indexer.AliasValidation) *response.Problem {
	detail := validation.Detail
	if detail == "" {
		detail = fmt.Sprintf("alias %q is invalid", aliasName)
	}
	prob := response.New(http.StatusBadRequest, "alias invalid",
		response.WithExtension("code", validation.Code),
		response.WithExtension("alias", aliasName),
		response.WithDetail(detail))
	return &prob
}

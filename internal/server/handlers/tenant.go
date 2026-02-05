package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
)

const defaultTenant = "default"

func resolveTenant(ctx context.Context, requestTenant string) (string, *response.Problem) {
	reqTenant := strings.TrimSpace(requestTenant)
	principalTenant, hasPrincipalTenant := requestctx.Tenant(ctx)
	_, hasPrincipal := requestctx.Principal(ctx)

	if hasPrincipalTenant {
		if reqTenant != "" && reqTenant != principalTenant {
			detail := fmt.Sprintf("request tenant %q does not match principal tenant %q", reqTenant, principalTenant)
			prob := response.New(http.StatusForbidden, "tenant mismatch",
				response.WithType(response.ProblemTypeTenantMismatch),
				response.WithExtension("code", response.ProblemCodeTenantMismatch),
				response.WithDetail(detail),
				response.WithExtension("request_tenant", reqTenant),
				response.WithExtension("principal_tenant", principalTenant),
			)
			return "", &prob
		}
		return principalTenant, nil
	}

	if reqTenant != "" {
		return reqTenant, nil
	}
	if hasPrincipal {
		return defaultTenant, nil
	}
	return defaultTenant, nil
}

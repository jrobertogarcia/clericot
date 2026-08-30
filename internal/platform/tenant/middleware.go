package tenant

import (
	"net/http"

	"clericot/internal/platform/auth"
)

// Middleware extracts and validates tenant scope from HTTP headers or authenticated principal.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		principal := auth.PrincipalFromContext(ctx)

		var targetTenantID string

		// 1. Check for explicit X-Tenant-ID header
		if headerTenant := r.Header.Get("X-Tenant-ID"); headerTenant != "" {
			if principal != nil && !principal.HasTenantAccess(headerTenant) {
				http.Error(w, `{"type":"forbidden","title":"Tenant Access Denied"}`, http.StatusForbidden)
				return
			}
			targetTenantID = headerTenant
		} else if principal != nil {
			// 2. Default to principal's assigned tenant
			targetTenantID = principal.TenantID
		}

		if targetTenantID != "" {
			ctx = WithTenant(ctx, targetTenantID)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

package auth

import (
	"context"
)

type principalKey struct{}

// AuthPrincipal represents an authenticated entity in the request context.
type AuthPrincipal struct {
	ID       string   `json:"id"`
	TenantID string   `json:"tenant_id"`
	Email    string   `json:"email"`
	Role     string   `json:"role"`
	Tenants  []string `json:"tenants,omitempty"`
}

// HasTenantAccess checks if the principal is authorized to access the given tenant.
func (p *AuthPrincipal) HasTenantAccess(targetTenantID string) bool {
	if p == nil {
		return false
	}
	if p.Role == "superadmin" || p.TenantID == targetTenantID {
		return true
	}
	for _, tid := range p.Tenants {
		if tid == targetTenantID {
			return true
		}
	}
	return false
}

// HasRole checks if the principal holds the requested role.
func (p *AuthPrincipal) HasRole(requiredRole string) bool {
	if p == nil {
		return false
	}
	if p.Role == "superadmin" {
		return true
	}
	return p.Role == requiredRole
}

// WithPrincipal stores the AuthPrincipal in context.
func WithPrincipal(ctx context.Context, p *AuthPrincipal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFromContext extracts the AuthPrincipal from context.
func PrincipalFromContext(ctx context.Context) *AuthPrincipal {
	if p, ok := ctx.Value(principalKey{}).(*AuthPrincipal); ok {
		return p
	}
	return nil
}

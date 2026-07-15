// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package tenant

import (
	"context"
	"net/http"
)

// Provider is the pluggable tenant resolution interface.
// Open-source default: SingleTenantProvider always returns system_default.
// Commercial extension: implement Provider with DB lookup, JWT claims, or header-based resolution.
type Provider interface {
	// Resolve extracts the tenant ID from an HTTP request.
	Resolve(r *http.Request) string
}

// SingleTenantProvider always returns the system default tenant.
// Used in the open-source single-user deployment mode.
type SingleTenantProvider struct{}

func (p *SingleTenantProvider) Resolve(r *http.Request) string { return "system_default" }

// HeaderProvider resolves tenant from X-Tenant-ID header.
// Used in commercial multi-tenant deployments with API gateway.
type HeaderProvider struct{ Header string }

func (p *HeaderProvider) Resolve(r *http.Request) string {
	if p.Header == "" { p.Header = "X-Tenant-ID" }
	if v := r.Header.Get(p.Header); v != "" { return v }
	return "system_default"
}

// Middleware injects tenant ID into the request context.
func Middleware(p Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tid := p.Resolve(r)
			ctx := context.WithValue(r.Context(), contextKey, tid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type contextKeyType string

const contextKey contextKeyType = "tenant_id"

// FromContext extracts the tenant ID from the request context.
func FromContext(r *http.Request) string {
	if v, ok := r.Context().Value(contextKey).(string); ok { return v }
	return "system_default"
}

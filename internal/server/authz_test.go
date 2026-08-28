package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fluxa/fluxa/internal/anchor"
	"github.com/fluxa/fluxa/internal/apikey"
	"github.com/fluxa/fluxa/internal/auth"
	"github.com/fluxa/fluxa/internal/batch"
	"github.com/fluxa/fluxa/internal/domain"
	"github.com/fluxa/fluxa/internal/fees"
	"github.com/fluxa/fluxa/internal/fiat"
	"github.com/fluxa/fluxa/internal/fx"
	"github.com/fluxa/fluxa/internal/org"
	"github.com/fluxa/fluxa/internal/reconcile"
	"github.com/fluxa/fluxa/internal/schedule"
	"github.com/fluxa/fluxa/internal/transfer"
	"github.com/fluxa/fluxa/internal/treasury"
	"github.com/fluxa/fluxa/internal/wallet"
	"github.com/fluxa/fluxa/internal/webhook"
)

var authzJWTSecret = []byte("test-secret-authz")

func newAuthzTestServer(t *testing.T) *Server {
	t.Helper()

	treasuryHandler := treasury.NewHandler(nil).WithMutationGate(RequireRole(domain.RoleOwner, domain.RoleAdmin))

	return New(
		auth.NewHandler(nil),
		org.NewHandler(nil),
		wallet.NewHandler(nil),
		transfer.NewHandler(nil),
		fx.NewHandler(nil),
		fiat.NewHandler(nil),
		fiat.NewAnchorHandler(nil),
		anchor.NewHandler(nil),
		fees.NewHandler(nil),
		reconcile.NewHandler(nil),
		apikey.NewHandler(nil),
		nil,
		webhook.NewHandler(nil),
		batch.NewHandler(nil),
		schedule.NewHandler(nil),
		treasuryHandler,
		authzJWTSecret,
		"0",
		nil,
	)
}

func mustToken(t *testing.T, role string) string {
	t.Helper()
	tok, err := auth.GenerateToken("user-1", "tenant-1", role, "user@example.com", "access", authzJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return tok
}

func doRequest(t *testing.T, srv *Server, method, path, role string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+mustToken(t, role))
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)
	return rec.Code
}

// TestAdminRoutesRequireOwnerOrAdmin verifies that every /v1/admin/* route
// rejects viewer and developer roles, and lets owner/admin credentials past
// the authorization layer (a non-403/401 response proves the request reached
// the handler, even if the handler itself then fails against nil services).
func TestAdminRoutesRequireOwnerOrAdmin(t *testing.T) {
	srv := newAuthzTestServer(t)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/admin/fees/collected"},
		{http.MethodGet, "/v1/admin/anchors"},
		{http.MethodPost, "/v1/admin/anchors"},
		{http.MethodGet, "/v1/admin/reconciliation/summary"},
		{http.MethodPost, "/v1/admin/reconciliation/run"},
		{http.MethodGet, "/v1/admin/treasury/balances"},
		{http.MethodPost, "/v1/admin/treasury/sweep"},
		{http.MethodPut, "/v1/admin/treasury/config"},
	}

	for _, rt := range routes {
		for _, role := range []string{domain.RoleViewer, domain.RoleDeveloper} {
			t.Run(rt.method+" "+rt.path+"/"+role, func(t *testing.T) {
				code := doRequest(t, srv, rt.method, rt.path, role)
				if code != http.StatusForbidden {
					t.Fatalf("role %q on %s %s: expected 403, got %d", role, rt.method, rt.path, code)
				}
			})
		}

		for _, role := range []string{domain.RoleAdmin, domain.RoleOwner} {
			t.Run(rt.method+" "+rt.path+"/"+role, func(t *testing.T) {
				code := doRequest(t, srv, rt.method, rt.path, role)
				if code == http.StatusForbidden || code == http.StatusUnauthorized {
					t.Fatalf("role %q on %s %s: expected to pass authorization, got %d", role, rt.method, rt.path, code)
				}
			})
		}
	}
}

// TestOperationalRoutesAllowDeveloper verifies the non-admin operational
// group still only blocks the read-only viewer role, not developer.
func TestOperationalRoutesAllowDeveloper(t *testing.T) {
	srv := newAuthzTestServer(t)

	code := doRequest(t, srv, http.MethodGet, "/v1/fx/rates", domain.RoleViewer)
	if code != http.StatusForbidden {
		t.Fatalf("viewer on mutating-capable group: expected 403, got %d", code)
	}

	for _, role := range []string{domain.RoleDeveloper, domain.RoleAdmin, domain.RoleOwner} {
		code := doRequest(t, srv, http.MethodGet, "/v1/fx/rates", role)
		if code == http.StatusForbidden || code == http.StatusUnauthorized {
			t.Fatalf("role %q on operational route: expected to pass authorization, got %d", role, code)
		}
	}
}

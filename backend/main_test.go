package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthHeaderMiddleware_MissingGatewayHeaders(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestAuthHeaderMiddleware_InvalidGatewayToken(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	req.Header.Set("X-Gateway-Token", "wrong-secret-token")
	req.Header.Set("X-Enforcement-Point", ExpectedEnforcementPoint)
	req.Header.Set("X-User-Username", "alice")
	req.Header.Set("X-User-Email", "alice@example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestAuthHeaderMiddleware_MissingUserContext(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	req.Header.Set("X-Gateway-Token", ExpectedGatewayToken)
	req.Header.Set("X-Enforcement-Point", ExpectedEnforcementPoint)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestProfileHandler_Success(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	req.Header.Set("X-Gateway-Token", ExpectedGatewayToken)
	req.Header.Set("X-Enforcement-Point", ExpectedEnforcementPoint)
	req.Header.Set("X-User-Username", "alice")
	req.Header.Set("X-User-Email", "alice@example.com")
	req.Header.Set("X-User-Role", "user")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp ProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if resp.User.Username != "alice" || resp.User.Email != "alice@example.com" || resp.User.Role != "user" {
		t.Fatalf("unexpected user identity in response: %+v", resp.User)
	}
}

func TestAdminHandler_NonAdminForbidden(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("X-Gateway-Token", ExpectedGatewayToken)
	req.Header.Set("X-Enforcement-Point", ExpectedEnforcementPoint)
	req.Header.Set("X-User-Username", "alice")
	req.Header.Set("X-User-Email", "alice@example.com")
	req.Header.Set("X-User-Role", "user")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestAdminHandler_AdminAllowed(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("X-Gateway-Token", ExpectedGatewayToken)
	req.Header.Set("X-Enforcement-Point", ExpectedEnforcementPoint)
	req.Header.Set("X-User-Username", "bob")
	req.Header.Set("X-User-Email", "bob@example.com")
	req.Header.Set("X-User-Role", "admin,user")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp AdminResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if resp.User.Username != "bob" {
		t.Fatalf("expected user bob, got %s", resp.User.Username)
	}
}

func TestHealthHandler(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestProfileHandler_KrakenD_Success(t *testing.T) {
	handler := SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	req.Header.Set("X-Gateway-Token", ExpectedGatewayToken)
	req.Header.Set("X-Enforcement-Point", ExpectedEnforcementPointKraken)
	req.Header.Set("X-User-Username", "alice")
	req.Header.Set("X-User-Email", "alice@example.com")
	req.Header.Set("X-User-Role", "user")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp ProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if resp.User.EnforcementPoint != ExpectedEnforcementPointKraken {
		t.Fatalf("expected enforcement point %s, got %s", ExpectedEnforcementPointKraken, resp.User.EnforcementPoint)
	}
}


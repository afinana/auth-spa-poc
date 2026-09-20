package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type userContextKey string

const userCtxKey userContextKey = "user_identity"

const (
	ExpectedGatewayToken     = "aitana-poc-gateway-secret-token"
	ExpectedEnforcementPoint = "Kong-APIM-Boundary"
)

type UserIdentity struct {
	Username         string `json:"username"`
	Email            string `json:"email"`
	Role             string `json:"role"`
	GatewayToken     string `json:"gatewayToken"`
	EnforcementPoint string `json:"enforcementPoint"`
}

type ProfileResponse struct {
	Message   string       `json:"message"`
	User      UserIdentity `json:"user"`
	Timestamp string       `json:"timestamp"`
}

type AdminResponse struct {
	Message      string       `json:"message"`
	User         UserIdentity `json:"user"`
	SystemStatus string       `json:"systemStatus"`
	Timestamp    string       `json:"timestamp"`
}

// AuthHeaderMiddleware enforces zero-trust boundary headers and extracts user identity
func AuthHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS preflight and basic headers for local dev testing
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Gateway-Token, X-Enforcement-Point, X-User-Username, X-User-Email, X-User-Role")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		gatewayToken := r.Header.Get("X-Gateway-Token")
		enforcementPoint := r.Header.Get("X-Enforcement-Point")

		// Validate mandatory anti-spoofing gateway headers
		if gatewayToken != ExpectedGatewayToken || enforcementPoint != ExpectedEnforcementPoint {
			log.Printf("[SECURITY] Rejected request from %s: invalid gateway headers (token=%q, enforcement=%q)",
				r.RemoteAddr, gatewayToken, enforcementPoint)
			http.Error(w, "Forbidden: Untrusted gateway boundary", http.StatusForbidden)
			return
		}

		username := r.Header.Get("X-User-Username")
		email := r.Header.Get("X-User-Email")

		if username == "" || email == "" {
			log.Printf("[SECURITY] Rejected request from gateway: missing user identity headers (user=%q, email=%q)",
				username, email)
			http.Error(w, "Unauthorized: Missing user context", http.StatusUnauthorized)
			return
		}

		user := UserIdentity{
			Username:         username,
			Email:            email,
			Role:             r.Header.Get("X-User-Role"),
			GatewayToken:     gatewayToken,
			EnforcementPoint: enforcementPoint,
		}

		// Store validated user identity in request context
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userCtxKey).(UserIdentity)
	if !ok {
		http.Error(w, "Server Error: user context missing", http.StatusInternalServerError)
		return
	}

	resp := ProfileResponse{
		Message:   fmt.Sprintf("Welcome %s (%s) [Role: %s]", user.Username, user.Email, user.Role),
		User:      user,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func AdminHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userCtxKey).(UserIdentity)
	if !ok {
		http.Error(w, "Server Error: user context missing", http.StatusInternalServerError)
		return
	}

	// RBAC check: verify user role contains admin
	roles := strings.Split(user.Role, ",")
	isAdmin := false
	for _, role := range roles {
		if strings.TrimSpace(role) == "admin" {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		log.Printf("[RBAC] User %s with roles [%s] denied access to admin resource", user.Username, user.Role)
		http.Error(w, "Forbidden: Insufficient privileges (admin role required)", http.StatusForbidden)
		return
	}

	resp := AdminResponse{
		Message:      fmt.Sprintf("Authorized admin access granted for %s", user.Username),
		User:         user,
		SystemStatus: "All zero-trust APIM boundary controls operational",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"UP","service":"go-backend"}`))
}

func SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Public healthcheck
	mux.HandleFunc("/healthz", HealthHandler)

	// Protected API endpoints
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/api/profile", ProfileHandler)
	protectedMux.HandleFunc("/api/admin", AdminHandler)

	// Wrap protected routes with zero-trust boundary middleware
	mux.Handle("/api/", AuthHeaderMiddleware(protectedMux))

	return mux
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := SetupRoutes()
	addr := ":" + port
	log.Printf("Starting Aitana Go microservice on port %s...", port)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

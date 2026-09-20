# Backend Microservice (Go) - Auth POC

This Go microservice (`go-backend`) serves as the upstream protected resource within the Zero-Trust Architecture Proof of Concept (PoC). It implements **Zero-Trust Boundary Verification**, **Identity Context Propagation**, and **Role-Based Access Control (RBAC)**.

---

## 1. Architecture & Policy Enforcement Point (PEP) Flow

Rather than having every microservice independently validate JWT cryptographic signatures, the architecture decouples validation at the **Kong API Gateway (PEP)** and enforces strict boundary verification at the microservice:

```
[Client / Angular SPA]
          │
          │ 1. Request with "Authorization: Bearer <JWT>"
          ▼
┌────────────────────────────────────────────────────────┐
│ Kong API Gateway (PEP - Port 8000)                    │
│  - Cryptographically verifies JWT via Keycloak JWKS   │
│  - Unpacks claims (username, email, roles)             │
│  - Strips original "Authorization" header              │
│  - Injects anti-spoofing gateway boundary headers      │
└────────────────────────────────────────────────────────┘
          │
          │ 2. Upstream request with internal identity headers
          ▼
┌────────────────────────────────────────────────────────┐
│ Go Backend Microservice (Port 8080)                   │
│  - AuthHeaderMiddleware checks boundary headers        │
│  - Injects UserIdentity into request context           │
│  - Handlers enforce endpoint-level RBAC (admin, etc.)  │
└────────────────────────────────────────────────────────┘

Direct Client Bypass to Port 8080:
[Malicious Client] ─── Direct HTTP call (:8080) ───► [Go Backend] ──► 403 Forbidden (Blocked)
```

---

## 2. Zero-Trust Boundary & Header Contract

The microservice never accepts unverified requests. Upstream requests forwarded by Kong must satisfy the following contract:

### Gateway Security Headers (Anti-Spoofing)
| Header | Expected Value | Purpose |
| :--- | :--- | :--- |
| `X-Gateway-Token` | `aitana-poc-gateway-secret-token` | Proves request passed through the authenticated gateway boundary |
| `X-Enforcement-Point` | `Kong-APIM-Boundary` | Confirms PEP origin |

If either header is missing or incorrect, `AuthHeaderMiddleware` logs a security warning and immediately returns **`403 Forbidden: Untrusted gateway boundary`**.

### User Identity Headers
Kong’s `post-function` Lua plugin decodes the verified JWT and maps claims to downstream headers:

| Injected Header | Source JWT Claim | Example Value |
| :--- | :--- | :--- |
| `X-User-Username` | `preferred_username` or `sub` | `alice`, `bob` |
| `X-User-Email` | `email` | `alice@example.com` |
| `X-User-Role` | `roles` or `realm_access.roles` | `user` or `admin,user` |

If identity headers (`X-User-Username` or `X-User-Email`) are missing, the middleware returns **`401 Unauthorized: Missing user context`**.

---

## 3. Implementation Details

### Context Propagation (`AuthHeaderMiddleware`)
In `main.go`, the middleware validates headers and binds a typed `UserIdentity` struct to `r.Context()`:

```go
user := UserIdentity{
    Username:         username,
    Email:            email,
    Role:             r.Header.Get("X-User-Role"),
    GatewayToken:     gatewayToken,
    EnforcementPoint: enforcementPoint,
}

ctx := context.WithValue(r.Context(), userCtxKey, user)
next.ServeHTTP(w, r.WithContext(ctx))
```

### Endpoints & Authorization Rules

| Endpoint | Method | Access Level | Description |
| :--- | :--- | :--- | :--- |
| `/healthz` | `GET` | **Public** | Healthcheck for Docker/Kubernetes liveness probes. |
| `/api/profile` | `GET` | **Authenticated User** | Returns the caller's identity context and server timestamp. |
| `/api/admin` | `GET` | **Admin Only** | Checks that `user.Role` contains the `admin` role. Returns `403 Forbidden` for non-admin users. |

#### RBAC Verification Logic in `AdminHandler`
```go
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
```

---

## 4. Verification & Testing

### Run Unit Tests
The test suite covers missing gateway headers, invalid gateway tokens, missing identity context, valid profile requests, and RBAC enforcement:

```bash
cd backend
go test -v ./...
```

### Manual Testing with `curl`

#### 1. Test Gateway Bypass Prevention (Expect 403 Forbidden)
Directly hitting the microservice without gateway headers must be rejected:
```bash
curl -i http://localhost:8080/api/profile
# Output: HTTP/1.1 403 Forbidden: Untrusted gateway boundary
```

#### 2. Test Missing User Identity (Expect 401 Unauthorized)
```bash
curl -i \
  -H "X-Gateway-Token: aitana-poc-gateway-secret-token" \
  -H "X-Enforcement-Point: Kong-APIM-Boundary" \
  http://localhost:8080/api/profile
# Output: HTTP/1.1 401 Unauthorized: Missing user context
```

#### 3. Test Authorized Request via Kong Gateway (Expect 200 OK)
```bash
curl -i \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  http://localhost:8000/api/profile
```

#### 4. Test RBAC on Admin Endpoint
```bash
# As standard user (Alice - role: user) -> Expect 403
curl -i -H "Authorization: Bearer $ALICE_TOKEN" http://localhost:8000/api/admin

# As admin user (Bob - roles: admin,user) -> Expect 200
curl -i -H "Authorization: Bearer $BOB_TOKEN" http://localhost:8000/api/admin
```

---

## 5. Key File Reference

- [`main.go`](main.go): Server entry point, routing, `AuthHeaderMiddleware`, and handlers (`ProfileHandler`, `AdminHandler`, `HealthHandler`).
- [`main_test.go`](main_test.go): Comprehensive unit tests validating security boundary controls and RBAC.
- [`Dockerfile.backend`](Dockerfile.backend): Multi-stage container build producing a lightweight Alpine production image.
- [`../kong/kong.yml`](../kong/kong.yml): Kong configuration containing the `jwt` validation plugin and Lua `post-function` mapping claims to headers.

---

## 6. Running Locally

### Standalone (Go toolchain)
```bash
cd backend
PORT=8080 go run main.go
```

### Via Docker Compose
```bash
docker-compose up -d go-backend
```
Check health:
```bash
curl http://localhost:8080/healthz
# Output: {"status":"UP","service":"go-backend"}
```

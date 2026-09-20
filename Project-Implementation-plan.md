# Implementation Plan: Aitana Auth Architecture Proof-of-Concept (PoC)

Implement a contract-first, zero-trust APIM boundary architecture Proof-of-Concept adhering to [POC-Implementation-guide.md](file:///home/afinana/development/projects/auth-spa-poc/POC-Implementation-guide.md). The system strictly decouples user identity verification (**Keycloak IdP**) from access policy enforcement (**Kong API Gateway PEP**), providing sanitized, anti-spoofing assertion headers to a downstream **Go microservice**, consumed by an **Angular SPA** with OAuth 2.0 PKCE flow.

## Architecture & Security Review Required

> [!IMPORTANT]
> **Token Offloading & Header Assertion Boundary**:
>
> 1. Inbound requests from the Angular SPA carry `Authorization: Bearer <JWT>` to Kong (`:8000`).
> 2. Kong cryptographically verifies the JWT signature and expiration against Keycloak's public JWKS.
> 3. Kong strips the external raw `Authorization` header and injects validated claims (`X-User-Username`, `X-User-Email`, `X-User-Role`) alongside mandatory anti-spoofing boundary assertions (`X-Gateway-Token: aitana-poc-gateway-secret-token`, `X-Enforcement-Point: Kong-APIM-Boundary`).
> 4. The Go microservice rejects any direct requests missing or mismatched on these gateway boundary headers with `403 Forbidden`, remaining completely agnostic to OAuth/JWT mechanics.

> [!NOTE]
> **CORS & Gateway Port Layout**:
>
> - Keycloak IdP: `http://localhost:8081`
> - Kong PEP Gateway: `http://localhost:8000` (Proxy), `http://localhost:8001` (Admin)
> - Go Microservice: `http://localhost:8080` (Internal container `go-backend:8080`)
> - Angular SPA: `http://localhost:4200`
> Kong configuration will include CORS plugin rules for `http://localhost:4200` to allow preflight `OPTIONS` and headers.

---

## Proposed Changes

```
auth-spa-poc/
├── docker-compose.yml
├── keycloak/
│   └── realm-export.json
├── kong/
│   └── kong.yml
├── backend/
│   ├── go.mod
│   ├── main.go
│   ├── main_test.go
│   └── Dockerfile.backend
└── frontend/
    ├── package.json
    ├── angular.json
    ├── tsconfig.json
    ├── nginx.conf
    ├── Dockerfile.frontend
    └── src/
        ├── index.html
        ├── styles.css
        ├── main.ts
        └── app/
            ├── app.config.ts
            ├── app.component.ts
            ├── auth.service.ts
            └── auth.interceptor.ts
```

---

### Component 1: Keycloak Realm Configuration (`keycloak/`)

#### [NEW] [realm-export.json](file:///home/afinana/development/projects/auth-spa-poc/keycloak/realm-export.json)

- Realm: `auth-realm`
- Client: `angular-spa` (public client, PKCE enabled with S256 challenge, standard authorization code flow enabled, valid redirect URIs: `http://localhost:4200/*`, web origins: `http://localhost:4200`, `+`)
- Fixed RSA Keypair (or exported active key) so Kong can deterministically verify tokens offline and online
- Roles: `user`, `admin`
- Pre-provisioned test accounts for instant PoC evaluation:
  - `alice` (Password: `alice123`, Email: `alice@example.com`, Role: `user`)
  - `bob` (Password: `bob123`, Email: `bob@example.com`, Role: `admin`)
- Protocol mappers:
  - `preferred_username` mapped to access token
  - `email` mapped to access token
  - `roles` / `realm_access.roles` mapped to access token

---

### Component 2: Kong API Gateway PEP Configuration (`kong/`)

#### [NEW] [kong.yml](file:///home/afinana/development/projects/auth-spa-poc/kong/kong.yml)

- Declarative `_format_version: "3.0"` configuration
- Upstream service: `go-backend-service` -> `http://go-backend:8080`
- Route: `backend-route` matching `/api`
- Plugins:
  - `cors`: configured for `origins: ["http://localhost:4200"]`, methods `GET, POST, OPTIONS`, headers `Authorization, Content-Type, Accept`
  - `jwt`: verifies `exp` claim, matches consumer `jwt_secrets` key against Keycloak issuer `http://localhost:8081/realms/auth-realm` using Keycloak's public RSA key
  - `post-function`: extracts claims (`preferred_username`, `email`, `roles`), injects headers (`X-User-Username`, `X-User-Email`, `X-User-Role`, `X-Gateway-Token`, `X-Enforcement-Point`), and strips `Authorization` header

---

### Component 3: Go Microservice (`backend/`)

#### [NEW] [go.mod](file:///home/afinana/development/projects/auth-spa-poc/backend/go.mod)

- Module `aitana-auth-poc/backend` with standard library Go 1.24/1.26

#### [NEW] [main.go](file:///home/afinana/development/projects/auth-spa-poc/backend/main.go)

- `AuthHeaderMiddleware`:
  - Validates `X-Gateway-Token == "aitana-poc-gateway-secret-token"`
  - Validates `X-Enforcement-Point == "Kong-APIM-Boundary"`
  - Enforces presence of `X-User-Username` and `X-User-Email`
  - Propagates `UserIdentity` struct into request context
- `ProfileHandler`:
  - Returns greeting and JSON payload containing username, email, role, and boundary assertion metadata
- `AdminHandler`:
  - Role-based authorization check: requires role containing `admin` (demonstrating AuthZ)
- `HealthHandler`:
  - Returns `200 OK` for container readiness
- `main_test.go`:
  - Tests 403 Forbidden when gateway boundary headers are absent
  - Tests 401 Unauthorized when user context is missing
  - Tests 200 OK when valid gateway boundary and user context are present
  - Tests RBAC authorization on `/api/admin`

#### [NEW] [Dockerfile.backend](file:///home/afinana/development/projects/auth-spa-poc/backend/Dockerfile.backend)

- Multi-stage build producing a minimal scratch or alpine container exposing port 8080.

---

### Component 4: Angular Frontend SPA (`frontend/`)

#### [NEW] [package.json](file:///home/afinana/development/projects/auth-spa-poc/frontend/package.json), [angular.json](file:///home/afinana/development/projects/auth-spa-poc/frontend/angular.json), [tsconfig.json](file:///home/afinana/development/projects/auth-spa-poc/frontend/tsconfig.json)

- Angular 19 application using standalone components and `angular-oauth2-oidc`

#### [NEW] [src/app/auth.service.ts](file:///home/afinana/development/projects/auth-spa-poc/frontend/src/app/auth.service.ts)

- Configures `OAuthService` with PKCE authorization code flow:
  - Issuer: `http://localhost:8081/realms/auth-realm`
  - Redirect URI: `http://localhost:4200/index.html`
  - Client ID: `angular-spa`
  - Response type: `code`
  - Scope: `openid profile email roles`
- Methods for login, logout, getting user claims, tokens, and session state

#### [NEW] [src/app/auth.interceptor.ts](file:///home/afinana/development/projects/auth-spa-poc/frontend/src/app/auth.interceptor.ts)

- Attaches `Bearer <token>` strictly to requests destined for Kong Gateway (`http://localhost:8000`)
- Explicitly avoids attaching token to external or direct requests

#### [NEW] [src/app/app.component.ts](file:///home/afinana/development/projects/auth-spa-poc/frontend/src/app/app.component.ts), [src/app/app.component.html](file:///home/afinana/development/projects/auth-spa-poc/frontend/src/app/app.component.html)

- Rich UI with dark theme, glassmorphism cards, live APIM sequence flow diagram:
  - User identity badge (Username, Email, Assigned Roles)
  - Token details viewer with decoded JWT payload claims
  - Action cards:
    - **Login / Logout** (via Keycloak PKCE)
    - **Call Gateway API (`GET :8000/api/profile`)**: proves token offloading, header transformation, and successful backend response
    - **Test Direct Upstream Bypass (`GET :8080/api/profile`)**: demonstrates zero-trust boundary rejection (`403 Forbidden`)
    - **Test Admin Endpoint (`GET :8000/api/admin`)**: demonstrates Role-Based Access Control (succeeds for `bob`, denies for `alice`)
- Safe DOM handling (framework native bindings, no unescaped innerHTML, strict CSP compliant)

#### [NEW] [nginx.conf](file:///home/afinana/development/projects/auth-spa-poc/frontend/nginx.conf), [Dockerfile.frontend](file:///home/afinana/development/projects/auth-spa-poc/frontend/Dockerfile.frontend)

- Multi-stage Dockerfile: builds Angular app, serves with Nginx on port 80 with security headers (`X-Frame-Options`, `X-Content-Type-Options`, strict CSP).

---

### Component 5: Docker Compose Orchestration (`docker-compose.yml`)

#### [NEW] [docker-compose.yml](file:///home/afinana/development/projects/auth-spa-poc/docker-compose.yml)

- Services:
  - `postgres`: PostgreSQL 16 Alpine for Keycloak persistent store
  - `keycloak`: Keycloak 24.0 with `start-dev --import-realm` mounting `./keycloak/realm-export.json` on port 8081
  - `go-backend`: Go microservice building `./backend/Dockerfile.backend` on port 8080
  - `kong`: Kong 3.6 Alpine in DB-less mode mounting `./kong/kong.yml` on port 8000 (proxy) and 8001 (admin)
  - `frontend`: Angular SPA building `./frontend/Dockerfile.frontend` on port 4200
- Custom bridge network `auth-net`

---

## Verification Plan

### Automated Security Check

- **Security Scanner**: Run scan on newly created source files using `run_security_scanner` skill (if scanner backend is configured).
- **Security Audit**: Audit code against `mandatory-secure-web-skills` (XSS prevention, no tokens in localStorage, CSRF defense, anti-spoofing header verification, strict method allow-lists, secure cookies). Document results in `walkthrough.md`.

### Automated Go Backend Tests

```bash
cd backend && go test -v ./...
```

- Validate rejection of requests missing `X-Gateway-Token` and `X-Enforcement-Point` (HTTP 403)
- Validate rejection of requests missing user identity headers (HTTP 401)
- Validate successful profile resolution with valid boundary headers (HTTP 200)
- Validate RBAC enforcement on `/api/admin` (HTTP 200 for admin role, HTTP 403 for user role)

### Automated Integration Verification

```bash
# 1. Start all containers
docker compose up -d --build

# 2. Check container health status
docker compose ps

# 3. Test direct microservice bypass (Expect HTTP 403 Forbidden)
curl -i http://localhost:8080/api/profile

# 4. Test unauthenticated Kong gateway call (Expect HTTP 401 Unauthorized)
curl -i http://localhost:8000/api/profile

# 5. Acquire token via Keycloak password grant or PKCE test script and call Kong gateway (Expect HTTP 200 with user profile)
# 6. Verify role-based authorization for user vs admin accounts
```

### End-to-End Browser Flow Verification

- Using the `browser_subagent` tool:
  - Open `http://localhost:4200`
  - Click "Login with Keycloak" -> Verify redirection to Keycloak login page
  - Login as `alice` (`alice123`) -> Verify redirect back to Angular SPA with valid PKCE exchange
  - Click "Call Gateway API" -> Verify HTTP 200 response with `alice (alice@example.com) [Role: user]`
  - Click "Test Direct Bypass" -> Verify HTTP 403 rejection message displayed in UI
  - Click "Test Admin Endpoint" -> Verify HTTP 403 Forbidden (insufficient permissions)
  - Logout and log in as `bob` (`bob123`) -> Test Admin Endpoint -> Verify HTTP 200 Success!

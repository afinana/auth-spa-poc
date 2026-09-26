# Implementation Plan: Project Auth Architecture Proof-of-Concept (PoC)

This implementation plan defines the complete specification for the **Project Auth Architecture Proof-of-Concept (PoC)**. Built around a contract-first, zero-trust APIM boundary architecture, the design strictly decouples user identity verification (AuthN) from perimeter access policy enforcement (AuthZ) and downstream business logic, supporting **Keycloak** and **ZITADEL** as Identity Providers, **KrakenD**, **Kong**, **Tyk**, and **Apache APISIX** as Policy Enforcement Point (PEP) gateways, a downstream **Go microservice**, and an **Angular 19 SPA** with OAuth 2.0 PKCE flow.

---

## 1. Architecture & Security Review Required

> [!IMPORTANT]
> **Token Offloading & Header Assertion Boundary**:
>
> 1. Inbound requests from the Angular SPA carry `Authorization: Bearer <JWT>` to the PEP Gateway on port `:8000`.
> 2. The gateway cryptographically verifies the JWT signature and expiration against the IdP's public keys.
> 3. The gateway strips the external raw `Authorization` header and injects validated identity claims (`X-User-Username`, `X-User-Email`, `X-User-Role`) alongside mandatory anti-spoofing boundary assertions (`X-Gateway-Token: project-poc-gateway-secret-token`, `X-Enforcement-Point: <Gateway>-APIM-Boundary`).
> 4. The Go microservice rejects any direct requests missing or mismatched on these gateway boundary headers with `403 Forbidden`, remaining completely agnostic to OAuth/JWT mechanics.

> [!NOTE]
> **Port Layout Across Stacks**:
>
> - **Identity Provider (IdP)**: `http://localhost:8081` (Keycloak or ZITADEL OIDC adapter)
> - **PEP Gateway**: `http://localhost:8000` (KrakenD, Kong, Tyk, or APISIX)
> - **Kong Admin API**: `http://localhost:8001` (Kong stack only)
> - **Go Microservice**: `http://localhost:8080` (Direct container port `go-backend:8080`)
> - **Angular 19 SPA**: `http://localhost:4200`

---

## 2. Directory Structure & Components

```text
auth-spa-poc/
├── docker-compose.yml              # Complete environment orchestration (KrakenD API GW + Keycloak IdP)
├── docker-compose.krakend.yml      # Explicit KrakenD stack compose file
├── docker-compose.keycloak-kong.yml# Complete environment orchestration (Kong API GW + Keycloak IdP)
├── docker-compose.tyk.yml          # Complete environment orchestration (Tyk API GW + Keycloak IdP)
├── docker-compose.apisix.yml       # Complete environment orchestration (APISIX API GW + Keycloak IdP)
├── docker-compose.zitadel.yml      # Complete environment orchestration (Kong API GW + ZITADEL IdP)
├── README.md                       # Main documentation and access guide
├── docs/                           # Centralized documentation and implementation guides
│   ├── README.md                   # Documentation index & reading paths
│   ├── POC-Implementation-guide.md # Master architectural guide and requirements
│   ├── Project-Implementation-plan.md # Detailed implementation plan
│   ├── KONG-Implementation-Guide.md # Kong gateway deep dive
│   ├── KRAKEND-Implementation-Guide.md # KrakenD gateway deep dive
│   ├── TYK-Implementation-Guide.md # Tyk gateway deep dive
│   ├── APISIX-Implementation-Guide.md # APISIX gateway deep dive
│   └── ZITADEL-Implementation-Plan.md # ZITADEL IdP integration plan
├── keycloak/
│   └── realm-export.json           # Declarative Keycloak realm export with RS256 keypair
├── zitadel/
│   ├── bootstrap/                  # Automated provisioning and Kong key sync
│   └── steps.yaml                  # ZITADEL initialization configuration
├── krakend/
│   ├── README.md                   # KrakenD directory documentation
│   ├── krakend.json                # Declarative configuration for Keycloak IdP
│   └── krakend.zitadel.json        # Declarative configuration for ZITADEL IdP
├── kong/
│   ├── README.md                   # Kong directory documentation
│   ├── kong.yml                    # Declarative configuration for Keycloak IdP
│   └── kong.zitadel.yml            # Declarative configuration for ZITADEL IdP
├── tyk/
│   ├── README.md                   # Tyk directory documentation
│   ├── tyk.conf                    # Declarative headless gateway configuration
│   ├── apps/
│   │   └── app-backend.json        # API definition with JWT & CORS
│   ├── policies/
│   │   └── policies.json           # Headless security policies
│   └── middleware/
│       └── auth-transform.js       # JS middleware for JWT claims & header injection
├── apisix/
│   ├── README.md                   # APISIX configuration overview
│   ├── config.yaml                # APISIX standalone YAML provider setup
│   └── apisix.yaml                # API route and Keycloak OIDC policy
├── backend/
│   ├── README.md                   # Backend architecture, zero-trust headers & RBAC guide
│   ├── go.mod                      # Go module definition
│   ├── main.go                     # Go microservice with zero-trust middleware & RBAC
│   ├── main_test.go                # Unit test suite for boundary and authorization
│   └── Dockerfile.backend          # Multi-stage minimal container build
└── frontend/
    ├── README.md                   # Frontend architecture, JWT claims & PKCE guide
    ├── angular.json                # Angular CLI configuration
    ├── package.json                # Angular dependencies (including angular-oauth2-oidc)
    ├── nginx.conf                  # Nginx configuration with CSP and security headers
    ├── Dockerfile.frontend         # Nginx production container
    └── src/
        ├── index.html              # HTML shell with meta tags and fonts
        ├── styles.css              # Global styles, dark-mode theme, CSS variables
        ├── main.ts                 # Bootstrap entrypoint
        └── app/
            ├── app.config.ts       # Application providers & HTTP interceptors
            ├── app.component.ts    # Main component with IdP combo selector & test suite
            ├── app.component.html  # Responsive UI template with sequence flow & console
            ├── app.component.css   # Glassmorphism, animations, and status pills
            ├── auth.service.ts     # Multi-IdP PKCE service (Keycloak & ZITADEL)
            ├── zitadel.service.ts  # ZITADEL-specific claim & role utilities
            └── auth.interceptor.ts # Interceptor injecting Bearer tokens for API Gateway (:8000)
```

---

## 3. Implementation Components

### Component 1: Identity Providers (Keycloak & ZITADEL)

* **Keycloak (`keycloak/realm-export.json`)**:
  - Realm: `auth-realm`
  - Client: `angular-spa` (public client, PKCE enabled with S256 challenge)
  - Pre-provisioned accounts: `alice` (`alice123`, role: `user`), `bob` (`bob123`, role: `admin`, `user`)
  - Protocol mappers: `preferred_username`, `email`, `roles` mapped into access token.
* **ZITADEL (`zitadel/`)**:
  - Nginx OIDC compatibility adapter exposing port `8081`
  - Automated provisioning via `zitadel-bootstrap` container
  - Demo accounts: `alice` (`Alice123!`), `bob` (`Bob1234!`).

---

### Component 2: Interchangeable Policy Enforcement Points (PEPs)

1. **KrakenD Gateway (`krakend/krakend.json`)**:
   - Stateless Lura Go engine running on port `8000`
   - `auth/validator` dynamically retrieving and caching Keycloak JWKS (`/certs`)
   - `propagate_claims` forwarding `X-User-*` headers
   - `modifier/martian` injecting `X-Gateway-Token` and `X-Enforcement-Point: KrakenD-APIM-Boundary`.
2. **Kong Gateway (`kong/kong.yml`)**:
   - DB-less OpenResty engine on ports `8000` (proxy) and `8001` (admin)
   - `jwt` plugin verifying RS256 signature against Keycloak public key
   - `post-function` Lua script extracting claims and injecting `X-Gateway-Token` and `X-Enforcement-Point: Kong-APIM-Boundary`.
3. **Tyk Gateway (`tyk/`)**:
   - Headless Go engine on port `8000` backed by Redis (`tyk-redis:6379`)
   - File-based API definitions (`apps/app-backend.json`) and security policies (`policies/policies.json`)
   - RS256 JWT validation using Base64 RSA public key
   - JSVM middleware (`auth-transform.js`) injecting `X-Gateway-Token` and `X-Enforcement-Point: Tyk-APIM-Boundary`.
4. **Apache APISIX Gateway (`apisix/`)**:
   - File-driven standalone data plane on port `8000`, without etcd or an Admin API.
   - OIDC bearer-only plugin validating Keycloak tokens using discovery/JWKS and an issuer allowlist.
   - Lua `serverless-post-function` mapping authenticated claims, replacing trusted headers, and clearing token headers.

---

### Component 3: Go Microservice Backend (`backend/`)

- `AuthHeaderMiddleware`:
  - Validates `X-Gateway-Token == "project-poc-gateway-secret-token"`
  - Validates `isValidEnforcementPoint(ep)` for the configured Kong, KrakenD, Tyk, and APISIX boundary identifiers
  - Enforces presence of `X-User-Username` and `X-User-Email`
  - Injects `UserIdentity` struct into request context.
- Handlers:
  - `ProfileHandler`: returns user identity and boundary metadata
  - `AdminHandler`: enforces `admin` role check
  - `HealthHandler`: returns `200 OK` liveness.
- Test suite (`main_test.go`): verifies 403 when boundary headers are missing, 401 when user context is missing, 200 for valid calls through each supported gateway boundary, and 403/200 for RBAC checks.

---

### Component 4: Angular Frontend SPA (`frontend/`)

- Standalone Angular 19 client with PKCE authentication (`auth.service.ts`)
- IdP switcher dropdown in the navbar supporting Keycloak and ZITADEL
- Automatic Bearer token injection to `:8000` via `auth.interceptor.ts`
- Live APIM sequence flow visualization and test console
- Direct microservice bypass test button proving zero-trust rejection (`403 Forbidden`).

---

## 4. Verification & Testing

### Automated Go Backend Tests
```bash
cd backend && go test -v ./...
```

### Docker Compose Stack Launch
```bash
# KrakenD (Default)
docker compose up -d

# Kong
docker compose -f docker-compose.keycloak-kong.yml up -d

# Tyk
docker compose -f docker-compose.tyk.yml up -d

# APISIX
docker compose -f docker-compose.apisix.yml up -d

# ZITADEL
docker compose -f docker-compose.zitadel.yml up -d
```

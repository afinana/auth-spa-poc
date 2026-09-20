# Kong API Gateway Implementation Guide

This guide provides a comprehensive technical specification of **Kong Gateway (v3.6)** deployed as the perimeter Policy Enforcement Point (PEP) in the Aitana Auth Architecture. It details declarative DB-less mode, JWT validation, custom Lua post-functions for claim extraction, and anti-spoofing header injection.

---

## 1. Architectural Overview & Role

Kong operates at the perimeter of the architecture, sitting between the public client (Angular SPA on port `4200`) and the private internal microservices (Go backend on port `8080`).

```
+------------------------------------+
|     Angular SPA Frontend (:4200)   |
+------------------------------------+
                   |
                   | HTTP GET /api/* (Authorization: Bearer <JWT>)
                   v
+---------------------------------------------------------------------------------+
| Kong API Gateway PEP (:8000 proxy, :8001 admin)                                 |
| - Engine: OpenResty / Nginx + LuaJIT                                            |
| - Mode: DB-less Declarative (/usr/local/kong/declarative/kong.yml)             |
|                                                                                 |
| Plugin Chain:                                                                   |
|  1. cors           -> Intercepts preflights, validates Origin http://localhost:4200 |
|  2. jwt            -> Cryptographic signature verification (RS256 via public key) |
|  3. post-function  -> Lua script: extracts claims, injects X-User-* & X-Gateway-* |
|                       headers, strips Authorization header                      |
+---------------------------------------------------------------------------------+
                   |
                   | HTTP GET /api/* (Sanitized, Context-Enriched)
                   v
+---------------------------------------------------------------------------------+
| Go Microservice Backend (:8080)                                                 |
| - Validates: X-Gateway-Token == "aitana-poc-gateway-secret-token"               |
| - Validates: X-Enforcement-Point == "Kong-APIM-Boundary"                        |
| - Resolves: X-User-Username, X-User-Email, X-User-Role                           |
+---------------------------------------------------------------------------------+
```

---

## 2. Declarative Configuration Deep Dive

Kong runs in **DB-less mode** (`KONG_DATABASE: "off"`). All routes, services, consumers, and plugins are defined in `kong/kong.yml`.

### 2.1 Consumer & JWT Credential Mapping

Kong associates JWT credentials with a `consumer`. The `key` field corresponds to the token's `iss` (Issuer) claim:

```yaml
consumers:
  - username: aitana-client
    jwt_secrets:
      - key: "http://localhost:8081/realms/auth-realm"
        algorithm: "RS256"
        rsa_public_key: |
          -----BEGIN PUBLIC KEY-----
          MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxaDiMATVaDiupUmZdT22
          fG2tB7AoiWfdN1Gg1hqhsOaO3xo80NxVq3fKo/BGwVoVKg8X3+X4gTPLgEuFTB1/
          ...
          -----END PUBLIC KEY-----
```

* **Issuer Matching:** When Keycloak issues an access token, the `iss` claim is `http://localhost:8081/realms/auth-realm`. Kong looks up the credential where `key == iss`.
* **RS256 Public Key:** The RSA public key corresponds to the realm's active signing key exported from Keycloak.

### 2.2 Service & Route Configuration

```yaml
services:
  - name: go-backend-service
    url: http://go-backend:8080
    routes:
      - name: backend-route
        paths:
          - /api
        strip_path: false
```

* **`strip_path: false`**: Requests to `/api/profile` preserve `/api/profile` when proxied upstream to `go-backend:8080`.

### 2.3 Plugin Chain & Execution Sequence

#### Plugin 1: CORS (`cors`)
Configured to handle browser preflight `OPTIONS` requests before authentication:
```yaml
- name: cors
  config:
    origins:
      - "http://localhost:4200"
    methods:
      - GET
      - POST
      - PUT
      - DELETE
      - OPTIONS
      - HEAD
    headers:
      - Authorization
      - Content-Type
      - Accept
      - Origin
      - X-Requested-With
    credentials: true
    max_age: 3600
```

#### Plugin 2: JWT Verification (`jwt`)
Enforces cryptographic integrity:
```yaml
- name: jwt
  config:
    claims_to_verify:
      - exp
    run_on_preflight: false
```
* **`claims_to_verify: [exp]`**: Rejects expired tokens with `401 Unauthorized`.
* **`run_on_preflight: false`**: Ensures CORS preflights pass through to the `cors` plugin without requiring a JWT.

#### Plugin 3: Post-Function Lua Script (`post-function`)
Executes custom Lua code in the `access` phase to extract token claims and construct zero-trust boundary headers:
```yaml
- name: post-function
  config:
    access:
      - |
        local jwt_decoder = require("kong.plugins.jwt.jwt_parser")
        local auth_header = kong.request.get_header("authorization")
        if auth_header then
          local _, _, token = string.find(auth_header, "%s+(%S+)")
          if token then
            local jwt, err = jwt_decoder:new(token)
            if jwt and jwt.claims then
              local claims = jwt.claims
              kong.service.request.set_header("X-User-Username", claims.preferred_username or claims.sub or "unknown")
              kong.service.request.set_header("X-User-Email", claims.email or "")
              local roles = claims.roles or (claims.realm_access and claims.realm_access.roles) or {}
              if type(roles) == "table" then
                kong.service.request.set_header("X-User-Role", table.concat(roles, ","))
              else
                kong.service.request.set_header("X-User-Role", tostring(roles))
              end
            end
          end
        end
        kong.service.request.set_header("X-Gateway-Token", "aitana-poc-gateway-secret-token")
        kong.service.request.set_header("X-Enforcement-Point", "Kong-APIM-Boundary")
        kong.service.request.clear_header("authorization")
```

---

## 3. Operational Guide

### 3.1 Docker Compose Deployment
Kong is orchestrated via `docker-compose.keycloak-kong.yml`:

```bash
# Start the Keycloak + Kong stack
docker compose -f docker-compose.keycloak-kong.yml up -d

# Check status
docker compose -f docker-compose.keycloak-kong.yml ps
```

### 3.2 Port Allocations
| Port | Interface | Protocol | Description |
|---|---|---|---|
| `8000` | `0.0.0.0` | HTTP | Client traffic / API proxy entrypoint |
| `8001` | `0.0.0.0` | HTTP | Admin API (Declarative configuration status) |

### 3.3 Validating Declarative Configuration
Check the status of loaded declarative configurations via the Kong Admin API:
```bash
curl http://localhost:8001/status
curl http://localhost:8001/routes
curl http://localhost:8001/plugins
```

### 3.4 Verification Tests

#### Test 1: Missing Token (Expected 401)
```bash
curl -i http://localhost:8000/api/profile
# Expected: HTTP/1.1 401 Unauthorized
# {"message":"Unauthorized"}
```

#### Test 2: CORS Preflight (Expected 200/204)
```bash
curl -i -X OPTIONS http://localhost:8000/api/profile \
  -H "Origin: http://localhost:4200" \
  -H "Access-Control-Request-Method: GET" \
  -H "Access-Control-Request-Headers: Authorization"
# Expected: Access-Control-Allow-Origin: http://localhost:4200
```

#### Test 3: Authenticated Request
```bash
TOKEN="<JWT_FROM_KEYCLOAK>"
curl -i http://localhost:8000/api/profile -H "Authorization: Bearer $TOKEN"
# Expected: HTTP/1.1 200 OK
# Body contains: "enforcementPoint": "Kong-APIM-Boundary"
```

# KrakenD API Gateway Implementation Guide

This guide provides a comprehensive technical specification of **KrakenD Gateway (v2.7)** deployed as the perimeter Policy Enforcement Point (PEP) in the Aitana Auth Architecture. It details stateless declarative configuration, dynamic JWKS token validation, claim propagation, Martian header injection, and ultra-high-performance routing.

---

## 1. Architectural Overview & Role

KrakenD is a stateless, high-throughput, Go-based API gateway. Unlike traditional gateways that rely on external databases or Lua scripting runtimes, KrakenD processes requests via compiled Go pipeline components.

```
+------------------------------------+
|     Angular SPA Frontend (:4200)   |
+------------------------------------+
                   |
                   | HTTP GET /api/* (Authorization: Bearer <JWT>)
                   v
+---------------------------------------------------------------------------------+
| KrakenD API Gateway PEP (:8000)                                                 |
| - Engine: Lura (Go-based, completely stateless, no database)                    |
| - Configuration: /etc/krakend/krakend.json                                      |
|                                                                                 |
| Pipeline Components:                                                            |
|  1. security/cors     -> Evaluates preflights, sets CORS headers                |
|  2. auth/validator    -> Validates RS256 JWT against Keycloak dynamic JWKS       |
|                          Caches keys in-memory; maps claims to input headers    |
|  3. modifier/martian  -> Injects X-Gateway-Token & X-Enforcement-Point headers  |
|  4. proxy / no-op     -> Forwards pristine payloads to upstream microservice    |
+---------------------------------------------------------------------------------+
                   |
                   | HTTP GET /api/* (Sanitized, Context-Enriched)
                   v
+---------------------------------------------------------------------------------+
| Go Microservice Backend (:8080)                                                 |
| - Validates: X-Gateway-Token == "aitana-poc-gateway-secret-token"               |
| - Validates: X-Enforcement-Point == "KrakenD-APIM-Boundary"                     |
| - Resolves: X-User-Username, X-User-Email, X-User-Role                           |
+---------------------------------------------------------------------------------+
```

---

## 2. Declarative Configuration Deep Dive

KrakenD's behavior is fully configured via `krakend/krakend.json`.

### 2.1 Global Configuration & CORS
```json
{
  "version": 3,
  "name": "Aitana API Gateway (KrakenD)",
  "port": 8000,
  "timeout": "3000ms",
  "cache_ttl": "300s",
  "extra_config": {
    "security/cors": {
      "allow_origins": [
        "http://localhost:4200"
      ],
      "allow_methods": [
        "GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"
      ],
      "allow_headers": [
        "Authorization", "Content-Type", "Accept", "Origin", "X-Requested-With"
      ],
      "expose_headers": [
        "Content-Length", "Content-Type"
      ],
      "max_age": "12h",
      "allow_credentials": true
    }
  }
}
```
* **CORS at Gateway Edge:** All browser preflights are handled directly by the gateway without forwarding `OPTIONS` requests to downstream services.

### 2.2 Endpoint Definition: Token Validation & Claim Propagation

Each endpoint (e.g. `/api/profile` and `/api/admin`) explicitly declares its security validation:

```json
{
  "endpoint": "/api/profile",
  "method": "GET",
  "output_encoding": "no-op",
  "input_headers": [
    "X-User-Username",
    "X-User-Email",
    "X-User-Role"
  ],
  "extra_config": {
    "auth/validator": {
      "alg": "RS256",
      "jwk_url": "http://keycloak:8080/realms/auth-realm/protocol/openid-connect/certs",
      "disable_jwk_security": true,
      "cache": true,
      "propagate_claims": [
        ["preferred_username", "X-User-Username"],
        ["email", "X-User-Email"],
        ["roles", "X-User-Role"]
      ]
    }
  }
}
```

* **`output_encoding: "no-op"`**: Bypasses KrakenD's response aggregation engine, proxying the raw JSON response directly from the backend to the client.
* **`jwk_url`**: Dynamically retrieves and caches the JSON Web Key Set (JWKS) from Keycloak over the internal Docker network.
* **`disable_jwk_security: true`**: Permits HTTP communication between containers in local development.
* **`propagate_claims`**: Directly maps JWT claims into downstream HTTP headers before forwarding.
* **`input_headers`**: Declares that the generated headers are permitted to pass to the backend request pipeline.

### 2.3 Backend Upstream & Martian Header Injection

KrakenD uses Google's Martian library (`modifier/martian`) to inject zero-trust boundary headers:

```json
"backend": [
  {
    "url_pattern": "/api/profile",
    "encoding": "no-op",
    "host": [
      "http://go-backend:8080"
    ],
    "extra_config": {
      "modifier/martian": {
        "fifo.Group": {
          "scope": ["request"],
          "aggregateErrors": true,
          "modifiers": [
            {
              "header.Modifier": {
                "scope": ["request"],
                "name": "X-Gateway-Token",
                "value": "aitana-poc-gateway-secret-token"
              }
            },
            {
              "header.Modifier": {
                "scope": ["request"],
                "name": "X-Enforcement-Point",
                "value": "KrakenD-APIM-Boundary"
              }
            }
          ]
        }
      }
    }
  }
]
```

---

## 3. Operational Guide

### 3.1 Docker Compose Deployment
KrakenD is orchestrated via `docker-compose.krakend.yml` (or the default `docker-compose.yml`):

```bash
# Start the KrakenD stack
docker compose -f docker-compose.krakend.yml up -d

# Check status
docker compose -f docker-compose.krakend.yml ps
```

### 3.2 Port Allocations
| Port | Interface | Protocol | Description |
|---|---|---|---|
| `8000` | `0.0.0.0` | HTTP | Client traffic / API proxy entrypoint |

### 3.3 Verification Tests

#### Test 1: Missing Token (Expected 401)
```bash
curl -i http://localhost:8000/api/profile
# Expected: HTTP/1.1 401 Unauthorized
```

#### Test 2: CORS Preflight (Expected 200)
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
# Body contains: "enforcementPoint": "KrakenD-APIM-Boundary"
```

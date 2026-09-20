# Tyk API Gateway Implementation Guide

This guide provides a comprehensive technical specification of **Tyk Gateway (v5.3)** deployed as the perimeter Policy Enforcement Point (PEP) in the Aitana Auth Architecture. It details headless open-source configuration, Redis-backed session management, cryptographic JWT validation, and JavaScript Virtual Machine (JSVM) middleware for claim transformation and anti-spoofing injection.

---

## 1. Architectural Overview & Role

Tyk Gateway operates in **headless mode** without requiring the Tyk Dashboard or commercial license. It uses a lightweight Redis instance (`tyk-redis:6379`) for key storage, rate limiting, and session tracking, and runs file-based declarative API definitions and policies.

```text
+------------------------------------+
|     Angular SPA Frontend (:4200)   |
+------------------------------------+
                   |
                   | HTTP GET /api/* (Authorization: Bearer <JWT>)
                   v
+---------------------------------------------------------------------------------+
| Tyk API Gateway PEP (:8000)                                                     |
| - Engine: Go-based Tyk Core with Goja/Otto JSVM runtime                         |
| - Storage: Redis (tyk-redis:6379) for distributed state & rate-limiting         |
| - Configuration: /opt/tyk-gateway/tyk.conf                                      |
|                                                                                 |
| Pipeline Execution Order:                                                       |
|  1. CORS Middleware    -> Handles OPTIONS preflights, validates Origin 4200     |
|  2. Auth (enable_jwt)  -> Verifies RS256 signature against base64 RSA public key|
|                           Assigns default-policy session from policies.json    |
|  3. Post JS Middleware -> Executes auth-transform.js:                           |
|                           - Decodes JWT claims (username, email, roles)         |
|                           - Injects X-User-* downstream identity headers        |
|                           - Injects X-Gateway-Token & X-Enforcement-Point       |
|                           - Deletes incoming Authorization header               |
|  4. Reverse Proxy      -> Forwards request to target_url (go-backend:8080)      |
+---------------------------------------------------------------------------------+
                   |
                   | HTTP GET /api/* (Sanitized, Context-Enriched)
                   v
+---------------------------------------------------------------------------------+
| Go Microservice Backend (:8080)                                                 |
| - Validates: X-Gateway-Token == "aitana-poc-gateway-secret-token"               |
| - Validates: X-Enforcement-Point == "Tyk-APIM-Boundary"                         |
| - Resolves: X-User-Username, X-User-Email, X-User-Role                           |
+---------------------------------------------------------------------------------+
```

---

## 2. Headless Configuration & File Layout

Tyk reads configurations directly from mounted volumes:

```text
tyk/
├── tyk.conf                     # Server daemon configuration
├── apps/
│   └── app-backend.json         # API definition for /api/ routing
├── policies/
│   └── policies.json            # Headless security policies
└── middleware/
    └── auth-transform.js        # JSVM claims extraction & header injection
```

### 2.1 Gateway Server Configuration (`tyk.conf`)

```json
{
  "listen_port": 8000,
  "secret": "aitana-tyk-secret-key-352d20ee67be",
  "template_path": "/opt/tyk-gateway/templates",
  "app_path": "/opt/tyk-gateway/apps",
  "middleware_path": "/opt/tyk-gateway/middleware",
  "policies": {
    "policy_source": "file",
    "policy_record_name": "/opt/tyk-gateway/policies/policies.json"
  },
  "enable_jsvm": true,
  "use_db_app_configs": false,
  "storage": {
    "type": "redis",
    "host": "tyk-redis",
    "port": 6379,
    "database": 0,
    "optimisation_max_idle": 100
  },
  "enable_analytics": false,
  "hash_keys": false,
  "allow_insecure_certs": true
}
```

* **`listen_port: 8000`**: Matches the standard PEP port expected by the Angular SPA.
* **`policy_source: "file"`**: Instructs Tyk to load security policies from `policies/policies.json` without querying an external management API.
* **`enable_jsvm: true`**: Enables the embedded JavaScript Virtual Machine to execute `auth-transform.js`.

### 2.2 Security Policy (`policies/policies.json`)

Tyk requires authenticated keys to map to a valid policy record:

```json
{
  "default-policy": {
    "name": "Default Policy",
    "active": true,
    "rate": 10000,
    "per": 60,
    "quota_max": -1,
    "quota_renewal_rate": -1,
    "access_rights": {
      "backend-api": {
        "api_id": "backend-api",
        "api_name": "Aitana Backend API",
        "versions": ["Default"],
        "allowed_urls": []
      }
    }
  }
}
```

* **`access_rights`**: Explicitly authorizes access to `backend-api` (defined in `apps/app-backend.json`).

### 2.3 API Definition (`apps/app-backend.json`)

```json
{
  "name": "Aitana Backend API",
  "api_id": "backend-api",
  "org_id": "default",
  "use_keyless": false,
  "enable_jwt": true,
  "jwt_signing_method": "rsa",
  "jwt_source": "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2twaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUF4YURpTUFUVmFEaXVwVW1aZFQyMgpmRzJ0QjdBb2lXZmROMUdnMWhxaHNPYU8zeG84ME54VnEzZktvL0JHd1ZvVktnOFgzK1g0Z1RQTGdFdUZUQjEvCkZaMVdmdGQxQzlFWSthNFdHKzlqR09VME9MLzVmTjZPMFIzN3YzZ1VsRTBlbTJnL2dHOWZxYU5ZYy94a1YxMmsKMUJBVGxZenRyWnVVbEhVRlZIaUx1ak9INHY2anE0WElCNWlrbkZLbWU0NHNIakxjc2tqUFhMMUhsRE1kc2wvdQpEYXF5VzFPaktYTlBVQWl5ekNyL3NmcUJtZmpKODFPNWs0eUw2WDk4Zkp3OEJHTEFDUStXZDdpc05tbmRsOStOCjlMNFRiSElYQU1XZXJaYlhnWlhKVThsTUJWTnFRSXZoOGY5dEZCNHZvbzR2Y1Z2OU9OektLcExza3A3V203UWcKOVFJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0t",
  "jwt_identity_base_field": "preferred_username",
  "jwt_skip_kid": true,
  "jwt_default_policies": [
    "default-policy"
  ],
  "version_data": {
    "not_versioned": true,
    "versions": {
      "Default": {
        "name": "Default",
        "use_extended_paths": true
      }
    }
  },
  "proxy": {
    "listen_path": "/api/",
    "target_url": "http://go-backend:8080",
    "strip_listen_path": false
  },
  "CORS": {
    "enable": true,
    "allowed_origins": ["http://localhost:4200"],
    "allowed_methods": ["GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"],
    "allowed_headers": ["Authorization", "Content-Type", "Accept", "Origin", "X-Requested-With"],
    "exposed_headers": ["Content-Length", "Content-Type"],
    "allow_credentials": true,
    "max_age": 43200,
    "options_passthrough": false
  },
  "custom_middleware": {
    "post": [
      {
        "name": "authTransformMiddleware",
        "path": "/opt/tyk-gateway/middleware/auth-transform.js",
        "require_session": false
      }
    ]
  },
  "active": true
}
```

* **`jwt_source`**: Contains the Base64-encoded PEM public key issued by Keycloak. This ensures key validation happens locally without runtime HTTP calls or race conditions during boot.
* **`jwt_skip_kid: true`**: Keycloak issues tokens with a `kid` header; `jwt_skip_kid` instructs Tyk to validate against the provided public key directly without requiring dynamic key lookup by ID.
* **`jwt_default_policies: ["default-policy"]`**: Associates all successfully verified JWT tokens with the permissions declared in `policies.json`.
* **`custom_middleware.post`**: Registers `auth-transform.js` to run in the `post` phase (after JWT authentication completes).

### 2.4 JSVM Middleware (`middleware/auth-transform.js`)

```javascript
var authTransformMiddleware = new TykJS.TykMiddleware.NewMiddleware({});

authTransformMiddleware.NewProcessRequest(function(request, session) {
  // 1. Mandatory zero-trust APIM boundary security headers
  request.SetHeaders["X-Gateway-Token"] = "aitana-poc-gateway-secret-token";
  request.SetHeaders["X-Enforcement-Point"] = "Tyk-APIM-Boundary";

  // 2. Extract and decode Bearer JWT to populate downstream identity headers
  var authHeader = request.Headers["Authorization"] || request.Headers["authorization"];
  var authStr = "";
  if (Array.isArray(authHeader) && authHeader.length > 0) {
    authStr = authHeader[0];
  } else if (typeof authHeader === "string") {
    authStr = authHeader;
  }

  if (authStr) {
    var token = authStr.replace(/^Bearer\s+/i, "").trim();
    if (token) {
      try {
        var parts = token.split(".");
        if (parts.length >= 2) {
          var base64Url = parts[1];
          var base64 = base64Url.replace(/-/g, "+").replace(/_/g, "/");
          while (base64.length % 4) {
            base64 += "=";
          }
          var jsonPayload = b64dec(base64);
          var claims = JSON.parse(jsonPayload);

          var username = claims.preferred_username || claims.sub || "unknown";
          var email = claims.email || "";

          var roles = [];
          if (claims.roles) {
            roles = claims.roles;
          } else if (claims.realm_access && claims.realm_access.roles) {
            roles = claims.realm_access.roles;
          }

          var roleStr = Array.isArray(roles) ? roles.join(",") : String(roles || "");

          request.SetHeaders["X-User-Username"] = username;
          request.SetHeaders["X-User-Email"] = email;
          request.SetHeaders["X-User-Role"] = roleStr;
        }
      } catch (err) {
        log("[Aitana Auth Transform] Error decoding JWT claims: " + err);
      }
    }
  }

  // 3. Clear upstream Authorization header to prevent token leakage
  if (request.DeleteHeaders) {
    request.DeleteHeaders.push("Authorization");
    request.DeleteHeaders.push("authorization");
  }

  return authTransformMiddleware.ReturnData(request, session || {});
});
```

---

## 3. Operational Guide

### 3.1 Docker Compose Deployment

Tyk is orchestrated via `docker-compose.tyk.yml`:

```bash
# Start the Keycloak + Tyk stack
docker compose -f docker-compose.tyk.yml up -d

# Check status
docker compose -f docker-compose.tyk.yml ps
```

### 3.2 Port Allocations

| Port | Interface | Protocol | Description |
|---|---|---|---|
| `8000` | `0.0.0.0` | HTTP | Client traffic / API proxy entrypoint |
| `6379` | Internal | TCP | Tyk Redis storage service (`tyk-redis`) |

### 3.3 Gateway Health & Hello Check

```bash
curl http://localhost:8000/hello
# Expected:
# {"status":"pass","version":"v5.3.x","description":"Tyk GW"}
```

### 3.4 Verification Tests

#### Test 1: Missing Token (Expected 401)

```bash
curl -i http://localhost:8000/api/profile
# Expected: HTTP/1.1 401 Unauthorized
# {"error": "Authorization field missing"}
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
# Body contains: "enforcementPoint": "Tyk-APIM-Boundary"
```

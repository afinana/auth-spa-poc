# Tyk API Gateway Configuration (Headless Mode)

This directory contains the headless configuration files for **Tyk API Gateway** running as the Policy Enforcement Point (PEP) in front of the Go microservice backend.

## Directory Structure

```text
tyk/
├── tyk.conf                     # Tyk Gateway server configuration (headless mode)
├── apps/
│   └── app-backend.json         # API definition for /api/ routing to go-backend
├── policies/
│   └── policies.json            # Headless security policies
├── middleware/
│   └── auth-transform.js        # JavaScript middleware for JWT claims & header injection
└── README.md                    # This documentation
```

## Architecture & Security Enforcement

1. **Port & Routing**: Tyk listens on port `8000`. Requests to `http://localhost:8000/api/*` are proxied to `http://go-backend:8080/api/*`.
2. **CORS**: Full CORS support configured for `http://localhost:4200` with `allow_credentials: true`.
3. **JWT Authentication**: Enforces RS256 JWT signature verification using the RSA public key issued by Keycloak (`auth-realm`).
4. **JavaScript Middleware (`auth-transform.js`)**:
   - Injects mandatory anti-spoofing gateway headers:
     - `X-Gateway-Token: aitana-poc-gateway-secret-token`
     - `X-Enforcement-Point: Tyk-APIM-Boundary`
   - Decodes JWT claims to propagate user context downstream:
     - `X-User-Username`: Keycloak `preferred_username` or `sub`
     - `X-User-Email`: Keycloak `email`
     - `X-User-Role`: Keycloak `roles` or `realm_access.roles` (comma-separated)
   - Strips the client `Authorization` header to avoid upstream token exposure.

## Running Tyk with Keycloak

To start the full stack using Keycloak and Tyk:

```bash
docker compose -f docker-compose.tyk.yml up -d
```

To stop the stack:

```bash
docker compose -f docker-compose.tyk.yml down
```

## Detailed Documentation

For full architectural deep dives, JSVM internals, and testing procedures, refer to:

👉 **[Tyk Implementation Guide](../docs/TYK-Implementation-Guide.md)**

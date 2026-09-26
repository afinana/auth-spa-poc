# Apache APISIX Implementation Guide

This guide describes the Apache APISIX gateway integration for the Project Auth PoC. APISIX is the perimeter Policy Enforcement Point (PEP): it validates Keycloak access tokens, forwards only verified identity claims to the Go backend, and asserts the gateway boundary.

## Architecture

```text
Angular SPA (:4200)
  └─ Authorization: Bearer <Keycloak RS256 access token>
       ↓
Apache APISIX (:8000; container :9080)
  ├─ OIDC plugin validates JWT signature and issuer using Keycloak discovery/JWKS
  ├─ serverless-post-function maps validated claims to X-User-* headers
  ├─ injects X-Gateway-Token and X-Enforcement-Point
  └─ removes Authorization and OIDC token/userinfo headers
       ↓
Go backend (:8080)
  └─ validates the gateway boundary and applies /api/admin role authorization
```

The `openid-connect` plugin runs in bearer-only mode, so APISIX does not redirect users to log in: the Angular SPA retains the existing Authorization Code with PKCE flow. JWKS is fetched from the Keycloak container discovery endpoint, and the issuer is explicitly checked against `http://localhost:8081/realms/auth-realm`. CORS preflight `OPTIONS` requests skip OIDC and header transformation.

## Configuration

- `apisix/config.yaml` selects the data-plane YAML configuration provider and enables only the required `cors`, `openid-connect`, and `serverless-post-function` plugins.
- `apisix/apisix.yaml` configures `/api/*` routing to `go-backend:8080`.
- The placeholder OIDC `client_secret` is retained for APISIX 3.13 schema compatibility; bearer-only JWKS validation does not use it.
- The post-function reads the base64-encoded `X-Userinfo` emitted by the OIDC plugin, requires both username and email, and maps Keycloak direct/realm roles (and the ZITADEL role-map shape) to `X-User-Role`.
- The function overwrites incoming identity/boundary headers and clears the bearer token and OIDC helper headers before proxying. Missing or malformed authenticated user information fails closed with `401`.
- The backend accepts `APISIX-APIM-Boundary` alongside the existing gateway enforcement-point identifiers.

## Start and stop

Stop another stack first because all stacks use the same host ports:

```bash
docker compose -f docker-compose.apisix.yml up -d
docker compose -f docker-compose.apisix.yml ps
```

Stop the APISIX stack with:

```bash
docker compose -f docker-compose.apisix.yml down
```

## Verify

An unauthenticated API request must be rejected by APISIX:

```bash
curl -i http://localhost:8000/api/profile
```

An allowed CORS preflight must not require a bearer token:

```bash
curl -i -X OPTIONS http://localhost:8000/api/profile \
  -H "Origin: http://localhost:4200" \
  -H "Access-Control-Request-Method: GET" \
  -H "Access-Control-Request-Headers: Authorization"
```

After logging in through the Angular SPA, the profile call should succeed and report `APISIX-APIM-Boundary`; Alice should receive `403` from `/api/admin`, while Bob should receive `200`.

```bash
curl -i http://localhost:8000/api/profile -H "Authorization: Bearer <ACCESS_TOKEN>"
curl -i http://localhost:8000/api/admin -H "Authorization: Bearer <ACCESS_TOKEN>"
```

The API is intended for local PoC use. Replace demo secrets and configure TLS, secure key management, network isolation, and production IdP/JWKS settings before deploying outside a trusted development environment.

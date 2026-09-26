# Apache APISIX Configuration

This directory contains the file-driven, standalone APISIX configuration for the Keycloak stack. The `client_secret` value in the route is an unused placeholder for APISIX 3.13 schema compatibility; bearer-only verification uses JWKS and does not authenticate to the token endpoint.

- `config.yaml` enables standalone YAML configuration and the plugins used by this PoC.
- `apisix.yaml` defines API routing, CORS, Keycloak OIDC/JWKS validation, and trusted identity and boundary header injection.

APISIX listens on container port `9080`, published as `http://localhost:8000`. Configuration updates are loaded from `apisix.yaml`; the stack does not expose an Admin API or require etcd.

See the [APISIX Implementation Guide](../docs/APISIX-Implementation-Guide.md) for the full request flow, startup, and verification steps.

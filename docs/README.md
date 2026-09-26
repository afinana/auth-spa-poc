# Architecture & Implementation Guides

This directory contains the in-depth architectural guides, comparative benchmarks, and integration specifications for the **OAuth2/OIDC Single Page Application (SPA) with API Gateway Policy Enforcement Point (PEP)** Proof of Concept (PoC).

---

## 📚 Documentation Index

| Document | Scope | Gateway / IdP | Key Focus Areas |
| :--- | :--- | :--- | :--- |
| **[PoC Master Architecture Guide](POC-Implementation-guide.md)** | System Master Guide | All Gateways & IdPs | Zero-trust APIM boundary, comparative gateway evaluation matrix, network isolation, and PKCE flow. |
| **[Project Implementation Plan](Project-Implementation-plan.md)** | Delivery & Milestones | Multi-gateway PoC | Multi-phase roadmap, contract-first zero-trust gateway integration, and verification criteria. |
| **[Kong Implementation Guide](KONG-Implementation-Guide.md)** | Gateway PEP Deep Dive | Kong (v3.6) + Keycloak / ZITADEL | Declarative DB-less setup, JWT plugin, Lua claim transformation (`post-function`), anti-spoofing injection, Admin API. |
| **[KrakenD Implementation Guide](KRAKEND-Implementation-Guide.md)** | Gateway PEP Deep Dive | KrakenD (v2.7) + Keycloak / ZITADEL | Stateless Lura Go engine, dynamic JWKS caching (`auth/validator`), Martian request modifiers, ultra-low latency. |
| **[Tyk Implementation Guide](TYK-Implementation-Guide.md)** | Gateway PEP Deep Dive | Tyk (v5.3+) + Keycloak | Headless OSS setup, Redis session storage, RS256 JWT validation, JavaScript Virtual Machine (JSVM) middleware. |
| **[ZITADEL Implementation Plan](ZITADEL-Implementation-Plan.md)** | IdP Integration Plan | ZITADEL (v2.x) | Cloud-native multi-tenancy, dynamic key sync bootstrap, and comparative analysis vs. Keycloak. |

---

## 🏗️ Architectural Overview

The core pattern implemented across this repository enforces a **Zero-Trust Backend Boundary** via an API Gateway acting as a **Policy Enforcement Point (PEP)**:

```text
[ Angular SPA (Port 4200) ]
         │
         │  1. PKCE Auth Code Flow
         ▼
[ Identity Provider (Keycloak :8081 / ZITADEL :8080) ]
         │
         │  2. RS256 Access Token (JWT)
         ▼
[ API Gateway PEP (Port 8000: Kong / KrakenD / Tyk) ]
         │
         │  3. Validates Signature & Injects Secure Headers
         │     - X-Gateway-Token: project-poc-gateway-secret-token
         │     - X-Enforcement-Point: <Gateway-Specific-Identifier>
         │     - X-User-Username, X-User-Email, X-User-Role
         │  4. Strips raw client Authorization header
         ▼
[ Go Microservice Backend (Port 8080 / Internal) ]
   (Rejects any request missing valid gateway token or invalid enforcement point)
```

---

## 🚀 Quick Execution by Stack

| Gateway | IdP | Compose File | Startup Command |
| :--- | :--- | :--- | :--- |
| **KrakenD** | Keycloak | `docker-compose.krakend.yml` | `docker compose -f docker-compose.krakend.yml up -d` |
| **Kong** | Keycloak | `docker-compose.keycloak-kong.yml` | `docker compose -f docker-compose.keycloak-kong.yml up -d` |
| **Tyk** | Keycloak | `docker-compose.tyk.yml` | `docker compose -f docker-compose.tyk.yml up -d` |
| **Kong** | ZITADEL | `docker-compose.zitadel.yml` | `docker compose -f docker-compose.zitadel.yml up -d` |

---

## 📂 Component Guides

- 🖥️ **[Frontend Application Guide](../frontend/README.md)**: Angular 19 SPA, OAuth 2.0 with PKCE, `AuthInterceptor`, and decoded claims panel.
- ⚙️ **[Backend Microservice Guide](../backend/README.md)**: Go microservice, zero-trust header enforcement, RBAC handler (`/api/admin`), and test suite.
- 🦍 **[Kong Configuration Directory](../kong/README.md)**: DB-less declarative configurations (`kong.yml`, `kong.zitadel.yml`).
- 🐙 **[KrakenD Configuration Directory](../krakend/README.md)**: Declarative gateway definitions (`krakend.json`, `krakend.zitadel.json`).
- 🛡️ **[Tyk Configuration Directory](../tyk/README.md)**: Headless configuration, JSVM middleware, apps, and policies.

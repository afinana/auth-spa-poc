# **Aitana Auth Architecture Implementation Guide**

## **1\. Executive Summary & Architectural Overview**

&nbsp;

This specification defines the production-grade Proof of Concept (PoC) enterprise software architecture for **Aitana Auth**. Built around a contract-first, zero-trust APIM boundary architecture, the design strictly decouples user identity verification (AuthN) from access policy enforcement (AuthZ). **Keycloak** serves as the centralized Identity Provider (IdP), while **Kong API Gateway** acts as the Policy Enforcement Point (PEP) and perimeter gateway. Downstream microservices, such as the Go backend, remain completely agnostic to OAuth 2.0 protocol mechanics and JWT validation logic, relying exclusively on pre-validated user contexts and boundary assertion headers.

&nbsp;

* **AuthN Boundary (Keycloak IdP):** The Angular Single Page Application (SPA) redirects unauthenticated users to Keycloak using the OAuth 2.0 Authorization Code Flow with PKCE. Keycloak verifies user credentials, applies client scope mappings, and issues cryptographically signed JSON Web Tokens (JWT) containing normalized identity claims.  
* **Request Delegation (Angular SPA):** The Angular HTTP client intercepts outgoing backend requests, attaching the JWT access token in the \`Authorization: Bearer\` header strictly for calls destined for the Kong API Gateway proxy.  
* **PEP Enforcement & Header Injection (Kong Gateway):** Kong intercepts inbound requests at the perimeter, verifies JWT signatures and expirations against Keycloak's public JWKS endpoint, strips external raw \`Authorization\` headers, and injects validated claims into upstream user headers (\`X-User-Username\`, \`X-User-Email\`, \`X-User-Role\`) along with explicit gateway anti-spoofing assertion headers (\`X-Gateway-Token\`, \`X-Enforcement-Point\`).  
* **Backend Identity Resolution (Go Microservice):** The backend microservice executes middleware to validate mandatory anti-spoofing boundary headers before resolving user context from \`X-User-\*\` headers into native request contexts, enforcing contract-first business logic without token handling overhead.

##

## **2\. Core Architectural Concepts & Security Principles**

&nbsp;

**Authentication (AuthN) vs. Authorization (AuthZ):** This system enforces a strict separation of concerns between identity verification (AuthN) and policy enforcement (AuthZ) using a decoupled, contract-first, zero-trust APIM boundary architecture. Keycloak functions as the centralized Identity Provider (IdP) handling credential verification, multi-factor authentication, inbound client credentials, and identity federation. Kong operates as the Policy Enforcement Point (PEP) at the perimeter boundary, enforcing access policies before requests reach downstream services.

&nbsp;

**OAuth 2.0 Authorization Code Flow with PKCE:** Because Single Page Applications (Angular) run entirely in public client environments (the user's browser), client secrets cannot be securely stored. Utilizing the Authorization Code Flow with Proof Key for Code Exchange (PKCE) prevents authorization code interception attacks by dynamically binding code exchange requests with a cryptographically generated code verifier and code challenge pair.

&nbsp;

**API Gateway Pattern & Token Offloading:** Offloading JWT signature verification, JSON parsing, key rotation (JWKS caching), and claim assertion to the Kong API Gateway keeps downstream microservices lightweight and stateless. Microservices do not require OAuth client libraries or public key management, reducing code complexity and attack surface across the service fleet.

&nbsp;

**Trusted Header Injection & Anti-Spoofing Guardrails:** Kong sanitizes all incoming requests by stripping external raw \`Authorization\` tokens and injecting verified user identity claims into custom HTTP headers (\`X-User-Username\`, \`X-User-Email\`, \`X-User-Role\`). To establish explicit gateway trust and prevent header spoofing or direct upstream bypass, the gateway appends mandatory boundary assertion headers—\`X-Gateway-Token\` (a cryptographically signed gateway token) and \`X-Enforcement-Point\` (asserting the PEP origin). Downstream services validate these boundary headers before consuming user context.

&nbsp;

**Zero Trust Security Architecture & Network Defense-in-Depth:** Under Zero Trust principles, internal network communication is never inherently trusted. Kubernetes \`NetworkPolicy\` rules strictly enforce ingress filtering on the Go microservice pods, ensuring they only accept TCP traffic originating from Kong API Gateway instances, preventing bypass attacks within the cluster.

##

## **3\. End-to-End Sequence & Workflow Diagrams**

### **3.1 End-to-End Authentication & Request Flow**

The sequence below illustrates the end-to-end interactions between the client, identity provider, gateway, and backend microservice during user authentication and API invocation:

&nbsp;

```
[[ User / Angular SPA ] --( 1. Redirect to /auth with PKCE challenge )--> [ Keycloak IdP ]
[ Keycloak IdP ]     --( 2. Authenticate User & Return Auth Code )--> [ User / Angular SPA ]
[ User / Angular SPA ] --( 3. Exchange Auth Code + Code Verifier )----> [ Keycloak IdP ]
[ Keycloak IdP ]     --( 4. Issue Signed JWT Access Token )---------> [ User / Angular SPA ]
[ User / Angular SPA ] --( 5. REST Call: Bearer <JWT> )--------------> [ Kong API Gateway ]
[ Kong API Gateway ]   --( 6. Verify Signature/Claims & Transform )----> [ Kong Internal Engine ]
[ Kong API Gateway ]   --( 7. Proxy Request + X-User-* Headers )-------> [ Go Microservice ]
[ Go Microservice ]    --( 8. Process Business Logic & Respond )--------> [ User / Angular SPA ]
```

### **3.2 Network Security & Isolation Architecture**

&nbsp;

The architectural topology and boundary guardrail diagram below details perimeter ingress, edge routing, contract-first boundary verification, and pod-level isolation enforced via Kubernetes NetworkPolicies:

```
++-----------------------------------------------------------------------------------+
|                                External Public Internet                           |
|                    (Inbound Client Credentials / Identity Federation)            |
+---------------------------------------+-------------------------------------------+
                                        | HTTPS (Port 443) / OAuth 2.0 / OIDC
                                        v
+-----------------------------------------------------------------------------------+
| K8s Ingress / LoadBalancer Service (Kong Gateway PEP Ingress)                     |
+---------------------------------------+-------------------------------------------+
                                        | Internal Cluster Traffic
                                        v
+-----------------------------------------------------------------------------------+
| Kubernetes Namespace: auth-system                                                 |
|                                                                                   |
|  +---------------------+                 +-------------------------------------+  |
|  |  Kong Gateway Pods  |                 | Keycloak Identity Provider Pods     |  |
|  |  (PEP Boundary)     |                 | (Federation & Client Credentials)   |  |
|  +----------+----------+                 +-------------------------------------+  |
|             |                                                                     |
|             | Anti-Spoofing Headers (X-Gateway-Token, X-Enforcement-Point)        |
|             | Ingress Restricted by K8s NetworkPolicy (App: kong ONLY)            |
|             v                                                                     |
|  +-----------------------------------------------------------------------------+  |
|  | Go Microservice Pods (Contract-First Boundary Validation - Port 8080)       |  |
|  +-----------------------------------------------------------------------------+  |
+-----------------------------------------------------------------------------------+
```

##

## **4\. Keycloak Identity Setup Specification**

**1\. Create Realm and Client (Standard Auth Code Flow with PKCE):**

Create a new Realm named **auth-realm** for your application. Under **Clients**, create a new OpenID Connect client ID named **angular-spa** for the Angular SPA. Set **Client Authentication** to Off (Public Client) and enable the **Standard Flow** (Authorization Code Flow with PKCE). Configure valid Redirect URIs (e.g., **<http://localhost:4200/\>***, **<http://localhost:4200/index.html>**) and Web Origins/CORS (e.g., **<http://localhost:4200>**) to match your Angular host.

**2\. Enforce User Profile Attributes:**

Navigate to **Realm Settings** \> **User Profile** (enable the declarative user profile feature if using older versions). Ensure the **username** and **email** attributes are marked as **Required**. The password requirement is enforced by default in the authentication flow.

**3\. Configure Protocol Mappers for Claims:**

To ensure the necessary data reaches Kong, navigate to **Client Scopes** \> **roles** (or create a custom scope). Add mappers of type **User Property** for both **username** and **email**. Map them to token claim names (**preferred\_username** and **email**) and ensure **Add to access token** is enabled so Kong Gateway receives these claims.

**4\. Automated Realm Import for Docker Compose Demo:**

To achieve zero-touch configuration during local demonstration, export these configuration settings into a **realm-export.json** file. Mount this JSON file into the Keycloak container's import directory (\`/opt/keycloak/data/import/realm-export.json\`) in \`docker-compose.yml\` and pass the command argument **start-dev \--import-realm**. Keycloak will automatically load and configure the \`auth-realm\`, client, user profile rules, and protocol mappers on initialization upon running \`docker-compose up \-d\`.

##

## **5\. Kong API Gateway (PEP Boundary) Declarative**&nbsp;

##

## **Configuration**

**1.Define the Upstream and Service:**Routing.

Create an Upstream pointing to your Go microservice (e.g., go-backend:8080) and configure a corresponding Service and Route in Kong.

**2.Enable the JWT Plugin:**Token Validation.

Attach the jwt plugin to your Route or Service. You must configure Kong with the public key (or JWKS URI) of your Keycloak Realm so it can cryptographically verify the token signatures.

**3.Transform Claims to Headers:**Header Injection.

To extract claims, inject anti-spoofing headers, and strip external JWT tokens, configure Kong's transformation pipeline. In addition to mapping \`preferred\_username\` to \`X-User-Username\` and \`email\` to \`X-User-Email\`, Kong must inject \`X-Gateway-Token\` and \`X-Enforcement-Point\` header assertions to guarantee gateway trust downstream while handling inbound client credentials and identity federation from Keycloak.

Example Kong Declarative Configuration (kong.yml):

&nbsp;

```json
__format_version: "3.0"

services:
  - name: go-backend-service
    url: http://go-backend:8080
    routes:
      - name: backend-route
        paths:
          - /api
    plugins:
      - name: jwt
        config:
          claims_to_verify:
            - exp
      - name: request-transformer
        config:
          remove:
            headers:
              - Authorization
          add:
            headers:
              - "X-User-Username:$(headers.preferred_username)"
              - "X-User-Email:$(headers.email)"
              - "X-Gateway-Token:aitana-poc-gateway-secret-token"
              - "X-Enforcement-Point:Kong-APIM-Boundary"
```

##

## **6\. Angular Frontend Implementation Specification**

&nbsp;

Use the angular-oauth2-oidc library to handle the PKCE flow securely.

To implement the login page in Angular, configure the **OAuthService** during application startup and invoke the login flow on the login component:

&nbsp;

```ts
iimport { Component } from '@angular/core';
import { AuthConfig, OAuthService } from 'angular-oauth2-oidc';

export const authCodeFlowConfig: AuthConfig = {
  issuer: 'http://localhost:8081/realms/auth-realm',
  redirectUri: window.location.origin + '/index.html',
  clientId: 'angular-spa',
  responseType: 'code',
  scope: 'openid profile email roles',
  showDebugInformation: true
};

@Component({
  selector: 'app-login',
  template: '<button (click)="login()">Login with Keycloak</button>'
})
export class LoginComponent {
  constructor(private oauthService: OAuthService) {
    this.oauthService.configure(authCodeFlowConfig);
    this.oauthService.loadDiscoveryDocumentAndTryLogin();
  }

  login() {
    this.oauthService.initCodeFlow();
  }
}
```

&nbsp;

```ts
import { Injectable } from '@angular/core';
import { HttpInterceptor, HttpRequest, HttpHandler, HttpEvent } from '@angular/common/http';
import { OAuthService } from 'angular-oauth2-oidc';
import { Observable } from 'rxjs';

@Injectable()
export class AuthInterceptor implements HttpInterceptor {
  constructor(private oauthService: OAuthService) {}

  intercept(req: HttpRequest<any>, next: HttpHandler): Observable<HttpEvent<any>> {
    const token = this.oauthService.getAccessToken();

    // Only attach the token for requests destined for the Kong API Gateway
    if (token && req.url.startsWith('https://api.yourdomain.com')) {
      const clonedReq = req.clone({
        headers: req.headers.set('Authorization', `Bearer ${token}`)
      });
      return next.handle(clonedReq);
    }
    return next.handle(req);
  }
}
```

##

## **7\. Go Microservice Implementation Specification**

&nbsp;

The Go backend remains completely agnostic to Keycloak and JWTs. It relies on standard HTTP middleware to extract the trusted headers provided by Kong.

&nbsp;

```go
ppackage main

import (
 "context"
 "fmt"
 "net/http"
)

type userContextKey string

const userCtxKey userContextKey = "user_identity"

type UserIdentity struct {
 Username         string
 Email            string
 Role             string
 GatewayToken     string
 EnforcementPoint string
}

// Middleware to enforce zero-trust boundary headers and extract user identity
func AuthHeaderMiddleware(next http.Handler) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  gatewayToken := r.Header.Get("X-Gateway-Token")
  enforcementPoint := r.Header.Get("X-Enforcement-Point")

  // Validate mandatory anti-spoofing gateway headers
  if gatewayToken != "aitana-poc-gateway-secret-token" || enforcementPoint != "Kong-APIM-Boundary" {
   http.Error(w, "Forbidden: Untrusted gateway boundary", http.StatusForbidden)
   return
  }

  username := r.Header.Get("X-User-Username")
  email := r.Header.Get("X-User-Email")

  if username == "" || email == "" {
   http.Error(w, "Unauthorized: Missing user context", http.StatusUnauthorized)
   return
  }

  user := UserIdentity{
   Username:         username,
   Email:            email,
   Role:             r.Header.Get("X-User-Role"),
   GatewayToken:     gatewayToken,
   EnforcementPoint: enforcementPoint,
  }

  // Store validated user identity in the request context
  ctx := context.WithValue(r.Context(), userCtxKey, user)
  next.ServeHTTP(w, r.WithContext(ctx))
 })
}

func ProfileHandler(w http.ResponseWriter, r *http.Request) {
 user, ok := r.Context().Value(userCtxKey).(UserIdentity)
 if !ok {
  http.Error(w, "Server Error", http.StatusInternalServerError)
  return
 }
 fmt.Fprintf(w, "Welcome %s (%s) [Role: %s]", user.Username, user.Email, user.Role)
}

func main() {
 mux := http.NewServeMux()
 mux.HandleFunc("/api/profile", ProfileHandler)

 // Apply the middleware
 wrappedMux := AuthHeaderMiddleware(mux)
 http.ListenAndServe(":8080", wrappedMux)
}
```

##

## **8\. Containerization & Network Defense-in-Depth**

&nbsp;

To prevent attackers from bypassing Kong and spoofing the X-User-\* headers, strict network isolation is required.

If deploying to a Kubernetes distribution like K3s or GKE:

> 1. **Network Policies:** Implement Kubernetes NetworkPolicy resources that explicitly deny all ingress traffic to the Go microservice pods unless it originates from the Kong ingress controller pods.  
> 2. **Internal Routing:** Expose the Go microservices only via ClusterIP Services. Never expose them via NodePort or LoadBalancer. Kong should be the single entry point exposed to the public internet.

##

##

## **9\. Deployment & Gemini CLI Execution**

##

### **9\. Docker Compose Setup (docker-compose.yml)**

The following \`docker-compose.yml\` orchestrates all system components, including PostgreSQL, Keycloak, Kong API Gateway, Go Backend, and Angular SPA:

&nbsp;

```
vversion: '3.8'

networks:
  auth-net:
    driver: bridge

volumes:
  postgres_data:

services:
  postgres:
    image: postgres:16-alpine
    container_name: postgres
    environment:
      POSTGRES_DB: keycloak
      POSTGRES_USER: keycloak
      POSTGRES_PASSWORD: keycloak_pass
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - auth-net
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U keycloak"]
      interval: 5s
      timeout: 5s
      retries: 5

  keycloak:
    image: quay.io/keycloak/keycloak:24.0
    container_name: keycloak
    command: start-dev --import-realm
    environment:
      KC_DB: postgres
      KC_DB_URL: jdbc:postgresql://postgres:5432/keycloak
      KC_DB_USERNAME: keycloak
      KC_DB_PASSWORD: keycloak_pass
      KEYCLOAK_ADMIN: admin
      KEYCLOAK_ADMIN_PASSWORD: admin
    volumes:
      - ./keycloak/realm-export.json:/opt/keycloak/data/import/realm-export.json
    ports:
      - "8081:8080"
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - auth-net

  go-backend:
    build:
      context: ./backend
      dockerfile: Dockerfile.backend
    container_name: go-backend
    networks:
      - auth-net

  kong:
    image: kong:3.6-alpine
    container_name: kong
    environment:
      KONG_DATABASE: "off"
      KONG_DECLARATIVE_CONFIG: /usr/local/kong/declarative/kong.yml
      KONG_PROXY_ACCESS_LOG: /dev/stdout
      KONG_ADMIN_ACCESS_LOG: /dev/stdout
      KONG_PROXY_ERROR_LOG: /dev/stderr
      KONG_ADMIN_ERROR_LOG: /dev/stderr
      KONG_ADMIN_LISTEN: 0.0.0.0:8001
    volumes:
      - ./kong/kong.yml:/usr/local/kong/declarative/kong.yml
    ports:
      - "8000:8000"
      - "8001:8001"
    depends_on:
      - go-backend
    networks:
      - auth-net

  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile.frontend
    container_name: angular-frontend
    ports:
      - "4200:80"
    networks:
      - auth-net
```

###

### **9.2 Kubernetes Deployment & Zero-Trust Network Policies**

The unified Kubernetes manifest for deployment onto K3s / K3d clusters includes strict \`NetworkPolicy\` controls enforcing isolate ingress to the Go backend:

&nbsp;

```
apiVersion: v1
kind: Namespace
metadata:
  name: auth-system
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kong-declarative-config
  namespace: auth-system
data:
  kong.yml: |
    _format_version: "3.0"
    services:
      - name: go-backend-service
        url: http://go-backend.auth-system.svc.cluster.local:8080
        routes:
          - name: backend-route
            paths:
              - /api
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-backend
  namespace: auth-system
  labels:
    app: go-backend
spec:
  replicas: 2
  selector:
    matchLabels:
      app: go-backend
  template:
    metadata:
      labels:
        app: go-backend
    spec:
      containers:
      - name: go-backend
        image: go-backend:latest
        ports:
        - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: go-backend
  namespace: auth-system
spec:
  type: ClusterIP
  selector:
    app: go-backend
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-go-backend-from-kong-only
  namespace: auth-system
spec:
  podSelector:
    matchLabels:
      app: go-backend
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: kong
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kong
  namespace: auth-system
  labels:
    app: kong
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kong
  template:
    metadata:
      labels:
        app: kong
    spec:
      containers:
      - name: kong
        image: kong:3.6-alpine
        env:
        - name: KONG_DATABASE
          value: "off"
        - name: KONG_DECLARATIVE_CONFIG
          value: /usr/local/kong/declarative/kong.yml
        ports:
        - containerPort: 8000
        volumeMounts:
        - name: kong-config
          mountPath: /usr/local/kong/declarative
      volumes:
      - name: kong-config
        configMap:
          name: kong-declarative-config
---
apiVersion: v1
kind: Service
metadata:
  name: kong
  namespace: auth-system
spec:
  type: LoadBalancer
  selector:
    app: kong
  ports:
  - port: 80
    targetPort: 8000
```

&nbsp;

## **10\. CLI Execution & Validation Instructions**

To execute and validate this PoC using Gemini CLI or automated tooling, run the following sequential commands in your terminal workspace:

```shell
# 1. Clone repository and set up directory structure
mkdir -p aitana-auth-poc/{backend,frontend,kong}
cd aitana-auth-poc

# 2. Build and start local docker-compose environment
docker-compose up -d --build

# 3. Verify health of all services
docker-compose ps

# 4. Test zero-trust header enforcement (Expected: 403 Forbidden)
curl -i http://localhost:8080/api/profile

# 5. Test proxied call through Kong Gateway without auth (Expected: 401 Unauthorized)
curl -i http://localhost:8000/api/profile

# 6. Test full flow via Angular SPA at http://localhost:4200
```

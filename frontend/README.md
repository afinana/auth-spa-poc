# Frontend Single Page Application (Angular 19) - Auth POC

This Angular application serves as the user-facing client in the Zero-Trust Architecture Proof of Concept (PoC). It implements **OAuth 2.0 Authorization Code Flow with PKCE**, processes and displays **JWT claims**, and dispatches authenticated requests to the **Kong API Gateway (PEP)**.

---

## 1. Architecture & Authentication Flow

```
[Keycloak (Port 8081)]
       ▲
       │ 1. Authorization Code Flow with PKCE
       ▼
[Angular Frontend (Port 4200)] ── 2. Decodes Payload (User Info & Roles)
       │
       │ 3. Attaches "Authorization: Bearer <token>"
       ▼
[Kong PEP Gateway (Port 8000)] ── 4. Cryptographic Validation via JWKS
       │
       ▼
[Go Backend Microservice (Port 8080)]
```

1. **User Authentication:** The frontend initiates the login flow via Keycloak (`angular-spa` public client).
2. **Token Exchange:** Upon successful authentication, Keycloak issues an Access Token (JWT), ID Token, and Refresh Token.
3. **Client-Side Claim Inspection:** The Angular app decodes the access token payload to customize the UI.
4. **Secured API Invocations:** The HTTP interceptor injects the Bearer token into outgoing requests destined for the Kong API Gateway (`http://localhost:8000`).

---

## 2. JWT Claims Handling in the Frontend

### Keycloak Protocol Mappers
Keycloak is configured (`keycloak/realm-export.json`) with protocol mappers that embed custom claims into the JWT:
- `preferred_username`: Keycloak account username (e.g., `alice`, `bob`).
- `email`: User's registered email address.
- `roles` & `realm_access.roles`: Assigned realm roles (e.g., `admin`, `user`).

### Client-Side Claim Decoding
Located in `src/app/auth.service.ts`, the `getClaims()` method extracts and parses the Base64URL-encoded JWT payload without requiring a cryptographic secret:

```typescript
public getClaims(): DecodedClaims | null {
  const token = this.getAccessToken();
  if (!token) return null;

  const parts = token.split('.');
  if (parts.length < 2) return null;

  const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
  const jsonPayload = decodeURIComponent(
    atob(base64)
      .split('')
      .map(c => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
      .join('')
  );
  return JSON.parse(jsonPayload);
}
```

### UI Consumption
In `src/app/app.component.ts`:
- **Identity:** `preferred_username` and `email` are displayed on the user profile card.
- **Role-Based UI:** `getRoles()` parses `claims['roles']` or `claims['realm_access']['roles']` to conditionally display UI tabs, admin actions, and authorization controls.

### Automatic Token Injection (`AuthInterceptor`)
In `src/app/auth.interceptor.ts`, an Angular `HttpInterceptor` intercepts requests targeting Kong Gateway (`http://localhost:8000`) and attaches the Bearer token:

```typescript
if (req.url.startsWith('http://localhost:8000')) {
  const token = this.oauthService?.getAccessToken();
  if (token) {
    const clonedReq = req.clone({
      headers: req.headers.set('Authorization', `Bearer ${token}`)
    });
    return next.handle(clonedReq);
  }
}
```

---

## 3. JWT Validation: Frontend vs. Gateway/Backend

It is critical to distinguish between **client-side verification** and **cryptographic backend validation**:

| Validation Layer | Checks Performed | Mechanism | Cryptographically Trustworthy? |
| :--- | :--- | :--- | :--- |
| **Frontend App** (`AuthService`) | Token existence & expiration (`exp`) | `this.oauthService.hasValidAccessToken()` verifies `exp * 1000 > Date.now()` | ❌ **No.** Only used for UX/state management; client memory can be altered in DevTools. |
| **Kong PEP Gateway** (`:8000`) | Signature, Issuer (`iss`), Audience (`aud`), Expiry (`exp`) | Validates cryptographic signature against Keycloak's public keys (`/protocol/openid-connect/certs`) | ✅ **Yes.** Protects internal network and enforces Zero Trust boundary. |
| **Go Backend** (`:8080`) | Context headers / direct token check | Checks gateway-injected identity headers (`X-User`, `X-Roles`) or validates token | ✅ **Yes.** Acts as Policy Decision/Enforcement Point. |

---

## 4. Validating and Inspecting JWTs

### CLI: Decode Claims with `jq`
```bash
echo "$TOKEN" | jq -R 'split(".") | .[1] | @base64d | fromjson'
```

### CLI: Check Expiration Status
```bash
EXP=$(echo "$TOKEN" | jq -R 'split(".") | .[1] | @base64d | fromjson | .exp')
NOW=$(date +%s)
if [ "$EXP" -gt "$NOW" ]; then
  echo "Token is VALID (expires in $((EXP - NOW))s)"
else
  echo "Token is EXPIRED ($((NOW - EXP))s ago)"
fi
```

### Verify Keycloak JWKS Public Key
Verify that the token's signature matches the public keys published by Keycloak:
```bash
curl -s http://localhost:8081/realms/auth-realm/protocol/openid-connect/certs | jq .
```

---

## 5. Key File Reference

- [`src/app/auth.service.ts`](src/app/auth.service.ts): OAuth2/OIDC configuration, login/logout actions, and claim extraction.
- [`src/app/auth.interceptor.ts`](src/app/auth.interceptor.ts): HTTP Interceptor attaching Bearer token to API Gateway requests.
- [`src/app/app.component.ts`](src/app/app.component.ts): Component managing authentication state, decoded claims display, and API test actions.
- [`src/app/app.component.html`](src/app/app.component.html): Dashboard UI displaying user profile, active claims table, raw token view, and gateway endpoints.

---

## 6. Running Locally

### Prerequisites
- Node.js (v20+)
- Angular CLI (`npm install -g @angular/cli`)

### Commands
```bash
# Install dependencies
npm install

# Start development server
ng serve --port 4200
```
Navigate to `http://localhost:4200`. Ensure Keycloak (`:8081`) and Kong Gateway (`:8000`) are running via `docker-compose up -d`.

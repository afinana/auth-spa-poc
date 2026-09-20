import { Injectable } from '@angular/core';
import { AuthConfig, OAuthService } from 'angular-oauth2-oidc';
import { BehaviorSubject, Observable } from 'rxjs';

export type IdpType = 'keycloak' | 'zitadel';

export interface IdpConfig {
  id: IdpType;
  name: string;
  issuer: string;
  clientId: string;
  scope: string;
  responseType: string;
  showDebugInformation: boolean;
  requireHttps: boolean;
  strictDiscoveryDocumentValidation: boolean;
  skipIssuerCheck: boolean;
  sessionChecksEnabled: boolean;
}

export const IDP_CONFIGS: Record<IdpType, IdpConfig> = {
  keycloak: {
    id: 'keycloak',
    name: 'Keycloak IdP',
    issuer: 'http://localhost:8081/realms/auth-realm',
    clientId: 'angular-spa',
    scope: 'openid profile email roles',
    responseType: 'code',
    showDebugInformation: true,
    requireHttps: false,
    strictDiscoveryDocumentValidation: false,
    skipIssuerCheck: true,
    sessionChecksEnabled: false
  },
  zitadel: {
    id: 'zitadel',
    name: 'ZITADEL IdP',
    issuer: 'http://localhost:8081',
    // Zitadel Angular SPA client id; direct integration without adapter
    clientId: '391627259401797638',
    scope: 'openid profile email urn:zitadel:iam:org:project:roles',
    responseType: 'code',
    showDebugInformation: true,
    requireHttps: false,
    strictDiscoveryDocumentValidation: false,
    skipIssuerCheck: true,
    sessionChecksEnabled: false
  }
};

export interface DecodedClaims {
  preferred_username?: string;
  name?: string;
  email?: string;
  roles?: string[] | string;
  exp?: number;
  iat?: number;
  iss?: string;
  sub?: string;
  [key: string]: any;
}

@Injectable({
  providedIn: 'root'
})
export class AuthService {
  private selectedIdpSubject: BehaviorSubject<IdpType>;
  public selectedIdp$: Observable<IdpType>;

  private isAuthenticatedSubject = new BehaviorSubject<boolean>(false);
  public isAuthenticated$ = this.isAuthenticatedSubject.asObservable();

  private discoveryPromise: Promise<boolean> = Promise.resolve(false);

  constructor(private oauthService: OAuthService) {
    const savedIdp = (localStorage.getItem('selected_idp') as IdpType) || 'keycloak';
    const initialIdp = (savedIdp === 'zitadel' || savedIdp === 'keycloak') ? savedIdp : 'keycloak';
    this.selectedIdpSubject = new BehaviorSubject<IdpType>(initialIdp);
    this.selectedIdp$ = this.selectedIdpSubject.asObservable();

    this.configureForIdp(initialIdp);

    this.oauthService.events.subscribe(() => {
      this.isAuthenticatedSubject.next(this.isLoggedIn());
    });
  }

  public getSelectedIdp(): IdpType {
    return this.selectedIdpSubject.value;
  }

  public setIdp(idp: IdpType): void {
    if (this.selectedIdpSubject.value === idp) {
      return;
    }
    // Logout from current IdP and switch configuration
    this.oauthService.logOut();
    this.isAuthenticatedSubject.next(false);
    localStorage.setItem('selected_idp', idp);
    this.selectedIdpSubject.next(idp);
    this.configureForIdp(idp);
  }

  private configureForIdp(idp: IdpType): void {
    const config = IDP_CONFIGS[idp];
    const authConfig: AuthConfig = {
      issuer: config.issuer,
      redirectUri: window.location.origin,
      clientId: config.clientId,
      responseType: config.responseType,
      scope: config.scope,
      showDebugInformation: config.showDebugInformation,
      requireHttps: config.requireHttps,
      strictDiscoveryDocumentValidation: config.strictDiscoveryDocumentValidation,
      skipIssuerCheck: config.skipIssuerCheck,
      sessionChecksEnabled: config.sessionChecksEnabled
    };

    this.oauthService.configure(authConfig);
    this.discoveryPromise = this.oauthService.loadDiscoveryDocumentAndTryLogin()
      .then(() => {
        const valid = this.isLoggedIn();
        this.isAuthenticatedSubject.next(valid);
        if (valid && window.location.search.includes('code=')) {
          window.history.replaceState({}, document.title, window.location.pathname);
        }
        return true;
      })
      .catch(err => {
        console.warn(`[${config.name}] Discovery initialization note:`, err);
        return false;
      });
  }

  public async login(): Promise<void> {
    await this.discoveryPromise;
    this.oauthService.initCodeFlow();
  }

  public logout(): void {
    this.oauthService.logOut();
    this.isAuthenticatedSubject.next(false);
  }

  public isLoggedIn(): boolean {
    return this.oauthService.hasValidAccessToken() || this.oauthService.hasValidIdToken();
  }

  public getAccessToken(): string {
    return this.oauthService.getAccessToken() || this.oauthService.getIdToken();
  }

  public getUsername(): string {
    const claims = this.getClaims();
    return claims?.preferred_username || claims?.['name'] || claims?.['sub'] || 'Anonymous';
  }

  public getEmail(): string {
    const claims = this.getClaims();
    return claims?.email || 'N/A';
  }

  public getRoles(): string[] {
    const claims = this.getClaims();
    if (!claims) return [];

    const roleSet = new Set<string>();

    // 1. Direct roles array
    if (Array.isArray(claims['roles'])) {
      claims['roles'].forEach(r => roleSet.add(r));
    } else if (typeof claims['roles'] === 'string') {
      claims['roles'].split(',').forEach(r => roleSet.add(r.trim()));
    }

    // 2. Keycloak realm_access.roles
    if (claims['realm_access'] && Array.isArray(claims['realm_access']['roles'])) {
      claims['realm_access']['roles'].forEach((r: string) => roleSet.add(r));
    }

    // 3. Zitadel project roles claim: "urn:zitadel:iam:org:project:roles"
    const zitadelRoles = claims['urn:zitadel:iam:org:project:roles'];
    if (zitadelRoles && typeof zitadelRoles === 'object') {
      Object.keys(zitadelRoles).forEach(r => roleSet.add(r));
    }

    return Array.from(roleSet);
  }

  public getClaims(): DecodedClaims | null {
    let accessClaims: DecodedClaims = {};
    const rawAccessToken = this.oauthService.getAccessToken();
    if (rawAccessToken) {
      try {
        const parts = rawAccessToken.split('.');
        if (parts.length >= 2) {
          const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
          const jsonPayload = decodeURIComponent(
            atob(base64)
              .split('')
              .map(c => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
              .join('')
          );
          accessClaims = JSON.parse(jsonPayload);
        }
      } catch (e) {
        console.warn('Failed to parse access token payload as JWT:', e);
      }
    }

    const idClaims = (this.oauthService.getIdentityClaims() as DecodedClaims) || {};

    if (!rawAccessToken && !this.oauthService.getIdToken()) {
      return null;
    }

    const merged: DecodedClaims = {
      ...accessClaims,
      ...idClaims
    };

    // Make sure Zitadel roles from either token are preserved
    const zitadelRoles = accessClaims['urn:zitadel:iam:org:project:roles'] || idClaims['urn:zitadel:iam:org:project:roles'];
    if (zitadelRoles) {
      merged['urn:zitadel:iam:org:project:roles'] = zitadelRoles;
    }

    // Direct fallback for user mapping if preferred_username is missing
    if (!merged.preferred_username) {
      if (merged.sub === '391627256046288902') {
        merged.preferred_username = 'alice';
        merged.email = merged.email || 'alice@example.com';
      } else if (merged.sub === '391627257505906694') {
        merged.preferred_username = 'bob';
        merged.email = merged.email || 'bob@example.com';
      } else if (merged['name']) {
        merged.preferred_username = merged['name'];
      }
    }

    return merged;
  }
}

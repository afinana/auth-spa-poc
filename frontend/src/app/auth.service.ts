import { Injectable } from '@angular/core';
import { AuthConfig, OAuthService } from 'angular-oauth2-oidc';
import { BehaviorSubject } from 'rxjs';

export const authCodeFlowConfig: AuthConfig = {
  issuer: 'http://localhost:8081/realms/auth-realm',
  redirectUri: window.location.origin,
  clientId: 'angular-spa',
  responseType: 'code',
  scope: 'openid profile email roles',
  showDebugInformation: true,
  requireHttps: false,
  strictDiscoveryDocumentValidation: false,
  skipIssuerCheck: true,
  sessionChecksEnabled: false
};

export interface DecodedClaims {
  preferred_username?: string;
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
  private isAuthenticatedSubject = new BehaviorSubject<boolean>(false);
  public isAuthenticated$ = this.isAuthenticatedSubject.asObservable();
  private discoveryPromise: Promise<boolean>;

  constructor(private oauthService: OAuthService) {
    this.oauthService.configure(authCodeFlowConfig);
    this.discoveryPromise = this.oauthService.loadDiscoveryDocumentAndTryLogin()
      .then(() => {
        const valid = this.oauthService.hasValidAccessToken();
        this.isAuthenticatedSubject.next(valid);
        if (valid) {
          // Clean URL parameters after code exchange
          if (window.location.search.includes('code=')) {
            window.history.replaceState({}, document.title, window.location.pathname);
          }
        }
        return true;
      })
      .catch(err => {
        console.warn('OAuth discovery initialization note:', err);
        return false;
      });

    this.oauthService.events.subscribe(() => {
      this.isAuthenticatedSubject.next(this.oauthService.hasValidAccessToken());
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
    return this.oauthService.hasValidAccessToken();
  }

  public getAccessToken(): string {
    return this.oauthService.getAccessToken();
  }

  public getUsername(): string {
    const claims = this.getClaims();
    return claims?.preferred_username || claims?.['name'] || 'Anonymous';
  }

  public getEmail(): string {
    const claims = this.getClaims();
    return claims?.email || 'N/A';
  }

  public getRoles(): string[] {
    const claims = this.getClaims();
    if (!claims) return [];
    if (Array.isArray(claims['roles'])) {
      return claims['roles'];
    }
    if (typeof claims['roles'] === 'string') {
      return claims['roles'].split(',');
    }
    if (claims['realm_access'] && Array.isArray(claims['realm_access']['roles'])) {
      return claims['realm_access']['roles'];
    }
    return [];
  }

  public getClaims(): DecodedClaims | null {
    const token = this.getAccessToken();
    if (!token) return null;
    try {
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
    } catch {
      return null;
    }
  }
}

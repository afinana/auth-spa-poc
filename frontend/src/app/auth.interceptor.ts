import { Injectable, Injector } from '@angular/core';
import { HttpInterceptor, HttpRequest, HttpHandler, HttpEvent } from '@angular/common/http';
import { OAuthService } from 'angular-oauth2-oidc';
import { Observable } from 'rxjs';

@Injectable()
export class AuthInterceptor implements HttpInterceptor {
  private oauthService?: OAuthService;

  constructor(private injector: Injector) {}

  intercept(req: HttpRequest<any>, next: HttpHandler): Observable<HttpEvent<any>> {
    // Only attach bearer token to requests destined for the Kong API Gateway (:8000)
    // This breaks the DI circular dependency because OAuthService requests to Keycloak (:8081) bypass injection
    if (req.url.startsWith('http://localhost:8000')) {
      if (!this.oauthService) {
        this.oauthService = this.injector.get(OAuthService);
      }
      const token = this.oauthService?.getAccessToken();
      if (token) {
        const clonedReq = req.clone({
          headers: req.headers.set('Authorization', `Bearer ${token}`)
        });
        return next.handle(clonedReq);
      }
    }

    return next.handle(req);
  }
}

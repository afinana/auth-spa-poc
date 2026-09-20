import { ApplicationConfig, provideZoneChangeDetection } from '@angular/core';
import { provideHttpClient, withInterceptorsFromDi, HTTP_INTERCEPTORS } from '@angular/common/http';
import { OAuthModule, OAuthStorage, provideOAuthClient } from 'angular-oauth2-oidc';
import { AuthInterceptor } from './auth.interceptor';

// Custom storage implementation to avoid localStorage security restrictions
export class SessionStorage implements OAuthStorage {
  getItem(key: string): string | null {
    try {
      return sessionStorage.getItem(key);
    } catch (e) {
      console.error('Session storage error:', e);
      return null;
    }
  }

  removeItem(key: string): void {
    try {
      sessionStorage.removeItem(key);
    } catch (e) {
      console.error('Session storage error:', e);
    }
  }

  setItem(key: string, value: string): void {
    try {
      sessionStorage.setItem(key, value);
    } catch (e) {
      console.error('Session storage error:', e);
    }
  }
}

export const appConfig: ApplicationConfig = {
  providers: [
    provideZoneChangeDetection({ eventCoalescing: true }),
    provideHttpClient(withInterceptorsFromDi()),
    provideOAuthClient(),
    {
      provide: HTTP_INTERCEPTORS,
      useClass: AuthInterceptor,
      multi: true
    }
  ]
};

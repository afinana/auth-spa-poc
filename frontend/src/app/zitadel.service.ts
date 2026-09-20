import { Injectable } from '@angular/core';
import { IdpConfig, IDP_CONFIGS, IdpType } from './auth.service';

@Injectable({
  providedIn: 'root'
})
export class ZitadelService {
  public readonly config: IdpConfig = IDP_CONFIGS.zitadel;

  public getConsoleUrl(): string {
    return 'http://localhost:8081/ui/console';
  }

  public getRolesFromClaims(claims: any): string[] {
    if (!claims) return [];
    const roles: string[] = [];
    const zitadelRoles = claims['urn:zitadel:iam:org:project:roles'];
    if (zitadelRoles && typeof zitadelRoles === 'object') {
      roles.push(...Object.keys(zitadelRoles));
    }
    return roles;
  }
}

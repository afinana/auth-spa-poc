import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient, HttpErrorResponse, HttpResponse } from '@angular/common/http';
import { AuthService, DecodedClaims, IdpType, IDP_CONFIGS } from './auth.service';
import { ZitadelService } from './zitadel.service';

interface ApiResult {
  endpoint: string;
  method: string;
  status: number | string;
  statusText?: string;
  success: boolean;
  timeMs: number;
  data: any;
  headers?: { [key: string]: string };
}

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.css']
})
export class AppComponent implements OnInit {
  public title = 'Project Auth Architecture PoC';
  public isAuthenticated = false;
  public claims: DecodedClaims | null = null;
  public roles: string[] = [];
  public rawToken = '';

  public loading = false;
  public activeTab: 'overview' | 'claims' | 'response' = 'overview';
  public lastResult: ApiResult | null = null;

  public selectedIdp: IdpType = 'keycloak';
  public idpConfigs = IDP_CONFIGS;
  public activeGateway = 'PEP Gateway';

  constructor(
    public authService: AuthService,
    public zitadelService: ZitadelService,
    private http: HttpClient
  ) {}

  ngOnInit(): void {
    this.selectedIdp = this.authService.getSelectedIdp();

    this.authService.selectedIdp$.subscribe(idp => {
      this.selectedIdp = idp;
    });

    this.authService.isAuthenticated$.subscribe(isAuth => {
      this.isAuthenticated = isAuth;
      if (isAuth) {
        this.claims = this.authService.getClaims();
        this.roles = this.authService.getRoles();
        this.rawToken = this.authService.getAccessToken();
      } else {
        this.claims = null;
        this.roles = [];
        this.rawToken = '';
      }
    });
  }

  public onIdpChange(newIdp: IdpType): void {
    this.selectedIdp = newIdp;
    this.authService.setIdp(newIdp);
    this.lastResult = null;
  }

  public login(): void {
    this.authService.login();
  }

  public logout(): void {
    this.authService.logout();
    this.lastResult = null;
  }

  // 1. Call via PEP Gateway (Port 8000)
  public callGatewayProfile(): void {
    this.executeRequest('GET', 'http://localhost:8000/api/profile');
  }

  // 2. Call Admin Endpoint via PEP Gateway (Port 8000)
  public callGatewayAdmin(): void {
    this.executeRequest('GET', 'http://localhost:8000/api/admin');
  }

  // 3. Test Direct Microservice Bypass (Port 8080 - Expected 403)
  public callDirectBypass(): void {
    this.executeRequest('GET', 'http://localhost:8080/api/profile');
  }

  private executeRequest(method: string, url: string): void {
    this.loading = true;
    const startTime = performance.now();

    this.http.request(method, url, { observe: 'response' }).subscribe({
      next: (res: HttpResponse<any>) => {
        const timeMs = Math.round(performance.now() - startTime);
        const headers: { [key: string]: string } = {};
        res.headers.keys().forEach(k => {
          headers[k] = res.headers.get(k) || '';
        });

        this.lastResult = {
          endpoint: url,
          method,
          status: res.status,
          statusText: res.statusText,
          success: res.status >= 200 && res.status < 300,
          timeMs,
          data: res.body,
          headers
        };
        this.loading = false;
        this.activeTab = 'response';
      },
      error: (err: HttpErrorResponse) => {
        const timeMs = Math.round(performance.now() - startTime);
        const headers: { [key: string]: string } = {};
        if (err.headers) {
          err.headers.keys().forEach(k => {
            headers[k] = err.headers.get(k) || '';
          });
        }

        this.lastResult = {
          endpoint: url,
          method,
          status: err.status || 'Network Error / Blocked',
          statusText: err.statusText || 'Failed to reach boundary',
          success: false,
          timeMs,
          data: err.error || err.message,
          headers
        };
        this.loading = false;
        this.activeTab = 'response';
      }
    });
  }

  public getFormattedJson(data: any): string {
    if (typeof data === 'string') {
      try {
        const parsed = JSON.parse(data);
        return JSON.stringify(parsed, null, 2);
      } catch {
        return data;
      }
    }
    return JSON.stringify(data, null, 2);
  }
}

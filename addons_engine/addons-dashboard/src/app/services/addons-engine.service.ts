import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { InstalledAddon } from '../models/addon.model';
import { ConfigService } from './config.service';

@Injectable({
  providedIn: 'root'
})
export class AddonsEngineService {
  
  constructor(
    private http: HttpClient,
    private configService: ConfigService
  ) {}

  private get baseUrl(): string {
    return `${this.configService.config.addonsEngineUrl}/api/v1/addons`;
  }

  getAddons(query?: any): Observable<InstalledAddon[]> {
    let params = new HttpParams();
    if (query) {
      Object.keys(query).forEach(key => {
        params = params.set(key, query[key]);
      });
    }
    return this.http.get<InstalledAddon[]>(this.baseUrl, { params });
  }

  getAddon(id: string): Observable<InstalledAddon> {
    return this.http.get<InstalledAddon>(`${this.baseUrl}/${id}`);
  }

  installAddon(marketplaceId: string): Observable<InstalledAddon> {
    return this.http.post<InstalledAddon>(this.baseUrl, { marketplace_id: marketplaceId });
  }

  uninstallAddon(id: string): Observable<InstalledAddon> {
    return this.http.delete<InstalledAddon>(`${this.baseUrl}/${id}`);
  }

  updateAddon(id: string, data: any): Observable<InstalledAddon> {
    return this.http.patch<InstalledAddon>(`${this.baseUrl}/${id}`, data);
  }
}

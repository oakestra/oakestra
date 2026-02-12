import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { MarketplaceAddon } from '../models/addon.model';
import { ConfigService } from './config.service';

@Injectable({
  providedIn: 'root'
})
export class MarketplaceService {
  
  constructor(
    private http: HttpClient,
    private configService: ConfigService
  ) {}

  private get baseUrl(): string {
    return `${this.configService.config.marketplaceUrl}/api/v1/marketplace/addons`;
  }

  getAddons(query?: any): Observable<MarketplaceAddon[]> {
    let params = new HttpParams();
    if (query) {
      Object.keys(query).forEach(key => {
        params = params.set(key, query[key]);
      });
    }
    return this.http.get<MarketplaceAddon[]>(this.baseUrl, { params });
  }

  getAddon(id: string): Observable<MarketplaceAddon> {
    return this.http.get<MarketplaceAddon>(`${this.baseUrl}/${id}`);
  }

  createAddon(addon: MarketplaceAddon): Observable<MarketplaceAddon> {
    return this.http.post<MarketplaceAddon>(this.baseUrl, addon);
  }

  deleteAddon(id: string): Observable<void> {
    return this.http.delete<void>(`${this.baseUrl}/${id}`);
  }
}

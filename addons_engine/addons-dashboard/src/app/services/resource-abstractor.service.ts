import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { Hook, CustomResource } from '../models/addon.model';
import { ConfigService } from './config.service';

@Injectable({
  providedIn: 'root'
})
export class ResourceAbstractorService {
  
  constructor(
    private http: HttpClient,
    private configService: ConfigService
  ) {}

  private get baseUrl(): string {
    return `${this.configService.config.resourceAbstractorUrl}/api/v1`;
  }

  // Hooks API
  getHooks(): Observable<Hook[]> {
    return this.http.get<Hook[]>(`${this.baseUrl}/hooks`);
  }

  getHook(id: string): Observable<Hook> {
    return this.http.get<Hook>(`${this.baseUrl}/hooks/${id}`);
  }

  createHook(hook: Hook): Observable<Hook> {
    return this.http.post<Hook>(`${this.baseUrl}/hooks`, hook);
  }

  updateHook(id: string, hook: Partial<Hook>): Observable<Hook> {
    return this.http.patch<Hook>(`${this.baseUrl}/hooks/${id}`, hook);
  }

  deleteHook(id: string): Observable<void> {
    return this.http.delete<void>(`${this.baseUrl}/hooks/${id}`);
  }

  // Custom Resources API
  getCustomResources(): Observable<CustomResource[]> {
    return this.http.get<CustomResource[]>(`${this.baseUrl}/custom-resources`);
  }

  createCustomResource(resource: CustomResource): Observable<CustomResource> {
    return this.http.post<CustomResource>(`${this.baseUrl}/custom-resources`, resource);
  }

  getResourcesByType(resourceType: string): Observable<any[]> {
    return this.http.get<any[]>(`${this.baseUrl}/custom-resources/${resourceType}`);
  }

  createResourceInstance(resourceType: string, data: any): Observable<any> {
    return this.http.post<any>(`${this.baseUrl}/custom-resources/${resourceType}`, data);
  }

  getResourceInstance(resourceType: string, id: string): Observable<any> {
    return this.http.get<any>(`${this.baseUrl}/custom-resources/${resourceType}/${id}`);
  }

  updateResourceInstance(resourceType: string, id: string, data: any): Observable<any> {
    return this.http.patch<any>(`${this.baseUrl}/custom-resources/${resourceType}/${id}`, data);
  }

  deleteResourceInstance(resourceType: string, id: string): Observable<void> {
    return this.http.delete<void>(`${this.baseUrl}/custom-resources/${resourceType}/${id}`);
  }
}

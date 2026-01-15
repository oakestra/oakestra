import { Injectable } from '@angular/core';
import { AppConfig } from '../models/addon.model';

@Injectable({
  providedIn: 'root'
})
export class ConfigService {
  private readonly CONFIG_KEY = 'oakestra-config';
  private readonly CONFIG_URL = '/assets/config.json';

  config: AppConfig = {
    marketplaceUrl: 'http://localhost:11102',
    addonsEngineUrl: 'http://localhost:11101',
    resourceAbstractorUrl: 'http://localhost:10000'
  };

  private initialized = false;

  constructor() {}

  async loadConfig(): Promise<void> {
    if (this.initialized) return;

    try {
      // First, try to load from environment config
      const response = await fetch(this.CONFIG_URL);
      if (response.ok) {
        const envConfig = await response.json();
        this.config = { ...this.config, ...envConfig };
      }
    } catch (error) {
      console.warn('Could not load environment config, using defaults', error);
    }

    // Then, override with user preferences from localStorage
    const saved = localStorage.getItem(this.CONFIG_KEY);
    if (saved) {
      const userConfig = JSON.parse(saved);
      this.config = { ...this.config, ...userConfig };
    }

    this.initialized = true;
  }

  saveConfig(config: AppConfig): void {
    this.config = config;
    localStorage.setItem(this.CONFIG_KEY, JSON.stringify(config));
  }

  resetToDefaults(): void {
    localStorage.removeItem(this.CONFIG_KEY);
    this.initialized = false;
    this.loadConfig();
  }
}

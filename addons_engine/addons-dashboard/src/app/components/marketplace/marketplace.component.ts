import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { MarketplaceService } from '../../services/marketplace.service';
import { AddonsEngineService } from '../../services/addons-engine.service';
import { MarketplaceAddon } from '../../models/addon.model';

@Component({
  selector: 'app-marketplace',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './marketplace.component.html',
  styleUrls: ['./marketplace.component.css']
})
export class MarketplaceComponent implements OnInit {
  addons: MarketplaceAddon[] = [];
  loading = false;
  error: string | null = null;
  showForm = false;
  selectedAddon: MarketplaceAddon | null = null;

  newAddon: MarketplaceAddon = {
    name: '',
    description: '',
    version: '',
    services: []
  };
  servicesJson = '';

  constructor(
    private marketplaceService: MarketplaceService,
    private addonsEngineService: AddonsEngineService
  ) {}

  ngOnInit(): void {
    this.loadAddons();
  }

  loadAddons(): void {
    this.loading = true;
    this.error = null;
    this.marketplaceService.getAddons().subscribe({
      next: (data) => {
        this.addons = data;
        this.loading = false;
      },
      error: (err) => {
        this.error = `Failed to load marketplace addons: ${err.message}`;
        this.loading = false;
      }
    });
  }

  showAddForm(): void {
    this.showForm = true;
    this.servicesJson = '[\n  {\n    "service_name": "my-service",\n    "image": "alpine",\n    "command": "echo hello"\n  }\n]';
  }

  cancelForm(): void {
    this.showForm = false;
    this.newAddon = { name: '', description: '', version: '', services: [] };
    this.servicesJson = '';
  }

  submitAddon(): void {
    try {
      this.newAddon.services = JSON.parse(this.servicesJson);
      this.marketplaceService.createAddon(this.newAddon).subscribe({
        next: () => {
          alert('✅ Addon added to marketplace successfully!');
          this.cancelForm();
          this.loadAddons();
        },
        error: (err) => alert(`❌ Error: ${err.message}`)
      });
    } catch (e: any) {
      alert(`❌ Invalid JSON: ${e.message}`);
    }
  }

  deleteAddon(id: string): void {
    if (!confirm('Are you sure you want to delete this addon from the marketplace?')) {
      return;
    }
    this.marketplaceService.deleteAddon(id).subscribe({
      next: () => {
        alert('✅ Addon deleted successfully!');
        this.loadAddons();
      },
      error: (err) => alert(`❌ Error: ${err.message}`)
    });
  }

  installAddon(id: string): void {
    this.addonsEngineService.installAddon(id).subscribe({
      next: () => {
        alert('✅ Addon installation started!');
      },
      error: (err) => alert(`❌ Error: ${err.message}`)
    });
  }

  viewDetails(addon: MarketplaceAddon): void {
    this.selectedAddon = addon;
  }

  closeDetails(): void {
    this.selectedAddon = null;
  }
}

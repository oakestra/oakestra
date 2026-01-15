import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AddonsEngineService } from '../../services/addons-engine.service';
import { InstalledAddon, AddonStatus } from '../../models/addon.model';

@Component({
  selector: 'app-installed-addons',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './installed-addons.component.html',
  styleUrls: ['./installed-addons.component.css']
})
export class InstalledAddonsComponent implements OnInit {
  addons: InstalledAddon[] = [];
  filteredAddons: InstalledAddon[] = [];
  loading = false;
  error: string | null = null;
  showForm = false;
  selectedAddon: InstalledAddon | null = null;
  statusFilter = 'all';
  marketplaceId = '';

  constructor(private addonsEngineService: AddonsEngineService) {}

  ngOnInit(): void {
    this.loadAddons();
  }

  loadAddons(): void {
    this.loading = true;
    this.error = null;
    this.addonsEngineService.getAddons().subscribe({
      next: (data) => {
        this.addons = data;
        this.applyFilter();
        this.loading = false;
      },
      error: (err) => {
        this.error = `Failed to load installed addons: ${err.message}`;
        this.loading = false;
      }
    });
  }

  applyFilter(): void {
    if (this.statusFilter === 'all') {
      this.filteredAddons = this.addons;
    } else {
      this.filteredAddons = this.addons.filter(addon => addon.status === this.statusFilter);
    }
  }

  setFilter(status: string): void {
    this.statusFilter = status;
    this.applyFilter();
  }

  showInstallForm(): void {
    this.showForm = true;
  }

  cancelForm(): void {
    this.showForm = false;
    this.marketplaceId = '';
  }

  installAddon(): void {
    this.addonsEngineService.installAddon(this.marketplaceId).subscribe({
      next: () => {
        alert('✅ Addon installation started!');
        this.cancelForm();
        setTimeout(() => this.loadAddons(), 1000);
      },
      error: (err) => alert(`❌ Error: ${err.message}`)
    });
  }

  uninstallAddon(id: string): void {
    if (!confirm('Are you sure you want to uninstall this addon?')) {
      return;
    }
    this.addonsEngineService.uninstallAddon(id).subscribe({
      next: () => {
        alert('✅ Addon uninstallation started!');
        setTimeout(() => this.loadAddons(), 1000);
      },
      error: (err) => alert(`❌ Error: ${err.message}`)
    });
  }

  viewDetails(addon: InstalledAddon): void {
    this.selectedAddon = addon;
  }

  closeDetails(): void {
    this.selectedAddon = null;
  }

  getStatusClass(status: string): string {
    return status.toLowerCase();
  }
}

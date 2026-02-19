import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AddonsEngineService } from '../../services/addons-engine.service';
import { NotificationService } from '../../services/notification.service';
import { ConfirmationService } from '../../services/confirmation.service';
import { InstalledAddon, AddonStatus } from '../../models/addon.model';
import { RefreshButtonComponent } from '../shared/refresh-button/refresh-button.component';

@Component({
  selector: 'app-installed-addons',
  standalone: true,
  imports: [CommonModule, FormsModule, RefreshButtonComponent],
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

  constructor(
    private addonsEngineService: AddonsEngineService,
    private notificationService: NotificationService,
    private confirmationService: ConfirmationService
  ) {}

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
      this.filteredAddons = this.addons.filter(addon => 
        addon.status && addon.status.toString() === this.statusFilter
      );
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
        this.notificationService.success('Addon installation started!');
        this.cancelForm();
        setTimeout(() => this.loadAddons(), 1000);
      },
      error: (err) => this.notificationService.error(`Error: ${err.message}`)
    });
  }

  async disableAddon(id: string): Promise<void> {
    const confirmed = await this.confirmationService.confirm({
      title: 'Disable Addon',
      message: 'Are you sure you want to disable this addon?\n\nYou can uninstall it later if needed.',
      confirmText: 'Disable',
      cancelText: 'Cancel'
    });

    if (!confirmed) {
      return;
    }

    this.addonsEngineService.uninstallAddon(id).subscribe({
      next: () => {
        this.notificationService.success('Addon disabled successfully!');
        setTimeout(() => this.loadAddons(), 1000);
      },
      error: (err) => this.notificationService.error(`Error: ${err.message}`)
    });
  }

  async uninstallAddon(id: string): Promise<void> {
    const confirmed = await this.confirmationService.confirm({
      title: 'Uninstall Addon',
      message: 'Are you sure you want to uninstall this addon?',
      confirmText: 'Uninstall',
      cancelText: 'Cancel'
    });

    if (!confirmed) {
      return;
    }

    this.addonsEngineService.uninstallAddon(id).subscribe({
      next: () => {
        this.notificationService.success('Addon uninstalled successfully!');
        setTimeout(() => this.loadAddons(), 1000);
      },
      error: (err) => this.notificationService.error(`Error: ${err.message}`)
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

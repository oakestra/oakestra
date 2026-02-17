import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterOutlet } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { ConfigService } from './services/config.service';
import { ConfirmationService } from './services/confirmation.service';
import { MarketplaceComponent } from './components/marketplace/marketplace.component';
import { InstalledAddonsComponent } from './components/installed-addons/installed-addons.component';
import { HooksComponent } from './components/hooks/hooks.component';
import { CustomResourcesComponent } from './components/custom-resources/custom-resources.component';
import { NotificationComponent } from './components/notification/notification.component';
import { ConfirmationDialogComponent } from './components/confirmation-dialog/confirmation-dialog.component';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [
    CommonModule,
    RouterOutlet,
    FormsModule,
    MarketplaceComponent,
    InstalledAddonsComponent,
    HooksComponent,
    CustomResourcesComponent,
    NotificationComponent,
    ConfirmationDialogComponent
  ],
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.css']
})
export class AppComponent implements OnInit {
  title = 'Oakestra Addons Management';
  activeTab = 'marketplace';
  showConfigDropdown = false;

  marketplaceUrl: string = '';
  addonsEngineUrl: string = '';
  resourceAbstractorUrl: string = '';

  constructor(
    public configService: ConfigService,
    private confirmationService: ConfirmationService
  ) {}

  ngOnInit(): void {
    // Config is already loaded via APP_INITIALIZER
    this.syncConfigValues();
  }

  syncConfigValues(): void {
    this.marketplaceUrl = this.configService.config.marketplaceUrl;
    this.addonsEngineUrl = this.configService.config.addonsEngineUrl;
    this.resourceAbstractorUrl = this.configService.config.resourceAbstractorUrl;
  }

  switchTab(tab: string): void {
    this.activeTab = tab;
  }

  toggleConfigDropdown(): void {
    this.showConfigDropdown = !this.showConfigDropdown;
  }

  closeConfigDropdown(): void {
    this.showConfigDropdown = false;
  }

  onConfigChange(): void {
    this.configService.saveConfig({
      marketplaceUrl: this.marketplaceUrl,
      addonsEngineUrl: this.addonsEngineUrl,
      resourceAbstractorUrl: this.resourceAbstractorUrl
    });
  }

  async resetToDefaults(): Promise<void> {
    const confirmed = await this.confirmationService.confirm({
      title: 'Reset Configuration',
      message: 'Reset configuration to defaults?\n\nThis will reload the page.',
      confirmText: 'Reset',
      cancelText: 'Cancel'
    });

    if (confirmed) {
      this.configService.resetToDefaults();
      window.location.reload();
    }
  }
}

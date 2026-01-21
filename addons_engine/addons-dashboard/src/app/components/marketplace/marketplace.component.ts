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
  
  // Form mode toggle - start in JSON mode
  useJsonMode = true;

  newAddon: MarketplaceAddon = {
    name: '',
    description: '',
    services: [],
    volumes: [],
    networks: []
  };
  
  // JSON mode
  addonJson = '';
  
  // Manual mode - for services
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
    this.useJsonMode = true;
    
    // Default JSON templates - using template literals for proper multiline strings
    this.servicesJson = `[
  {
    "service_name": "my-service",
    "image": "alpine:latest",
    "command": "echo hello",
    "ports": {"8080": "80"},
    "environment": {"KEY": "value"},
    "volumes": ["volume1:/data"],
    "networks": ["network1"]
  }
]`;
    
    this.addonJson = `{
  "name": "my-addon",
  "description": "My addon description",
  "services": [
    {
      "service_name": "my-service",
      "image": "alpine:latest",
      "command": "echo hello",
      "ports": {"8080": "80"},
      "environment": {"KEY": "value"},
      "volumes": ["volume1:/data"],
      "networks": ["network1"]
    }
  ],
  "volumes": [
    {
      "name": "volume1",
      "driver": "local"
    }
  ],
  "networks": [
    {
      "name": "network1",
      "driver": "bridge",
      "enable_ipv6": false
    }
  ]
}`;
  }

  cancelForm(): void {
    this.showForm = false;
    this.useJsonMode = true;
    this.newAddon = { 
      name: '', 
      description: '', 
      services: [],
      volumes: [],
      networks: []
    };
    this.servicesJson = '';
    this.addonJson = '';
  }

  submitAddon(): void {
    try {
      if (this.useJsonMode) {
        // JSON mode - parse entire addon object
        const addonData = JSON.parse(this.addonJson);
        this.marketplaceService.createAddon(addonData).subscribe({
          next: () => {
            alert('✅ Addon added to marketplace successfully!');
            this.cancelForm();
            this.loadAddons();
          },
          error: (err) => alert(`❌ Error: ${err.message}`)
        });
      } else {
        // Manual mode - parse only services
        this.newAddon.services = JSON.parse(this.servicesJson);
        this.marketplaceService.createAddon(this.newAddon).subscribe({
          next: () => {
            alert('✅ Addon added to marketplace successfully!');
            this.cancelForm();
            this.loadAddons();
          },
          error: (err) => alert(`❌ Error: ${err.message}`)
        });
      }
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

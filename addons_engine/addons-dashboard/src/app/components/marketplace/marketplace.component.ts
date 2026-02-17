import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { MarketplaceService } from '../../services/marketplace.service';
import { AddonsEngineService } from '../../services/addons-engine.service';
import { NotificationService } from '../../services/notification.service';
import { ConfirmationService } from '../../services/confirmation.service';
import { MarketplaceAddon, AddonService, AddonVolume, AddonNetwork } from '../../models/addon.model';
import { KeyValueInputComponent } from '../shared/key-value-input/key-value-input.component';
import { SimpleListInputComponent } from '../shared/simple-list-input/simple-list-input.component';
import { RefreshButtonComponent } from '../shared/refresh-button/refresh-button.component';

@Component({
  selector: 'app-marketplace',
  standalone: true,
  imports: [CommonModule, FormsModule, KeyValueInputComponent, SimpleListInputComponent, RefreshButtonComponent],
  templateUrl: './marketplace.component.html',
  styleUrls: ['./marketplace.component.scss']
})
export class MarketplaceComponent implements OnInit {
  addons: MarketplaceAddon[] = [];
  loading = false;
  error: string | null = null;
  showForm = false;
  selectedAddon: MarketplaceAddon | null = null;
  
  // Form mode toggle - start in manual mode
  useJsonMode = false;

  newAddon: MarketplaceAddon = {
    name: '',
    description: '',
    services: [],
    volumes: [],
    networks: []
  };
  
  // JSON mode
  addonJson = '';
  
  // Manual mode - UI driven
  currentService: AddonService = this.getEmptyService();
  currentVolume: AddonVolume = this.getEmptyVolume();
  currentNetwork: AddonNetwork = this.getEmptyNetwork();
  
  // Track editing state
  editingServiceIndex: number = -1;
  editingVolumeIndex: number = -1;
  editingNetworkIndex: number = -1;
  
  // Track add form visibility
  showServiceForm: boolean = false;
  showVolumeForm: boolean = false;
  showNetworkForm: boolean = false;

  constructor(
    private marketplaceService: MarketplaceService,
    private addonsEngineService: AddonsEngineService,
    private notificationService: NotificationService,
    private confirmationService: ConfirmationService
  ) {}

  ngOnInit(): void {
    this.loadAddons();
  }

  private getEmptyService(): AddonService {
    return {
      service_name: '',
      image: '',
      command: '',
      ports: {},
      environment: {},
      volumes: [],
      networks: [],
      labels: {}
    };
  }

  private getEmptyVolume(): AddonVolume {
    return {
      name: '',
      driver: 'local',
      driver_opts: {},
      labels: {}
    };
  }

  private getEmptyNetwork(): AddonNetwork {
    return {
      name: '',
      driver: 'bridge',
      enable_ipv6: false
    };
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
    this.useJsonMode = false;
    
    // Reset manual mode data
    this.currentService = this.getEmptyService();
    this.currentVolume = this.getEmptyVolume();
    this.currentNetwork = this.getEmptyNetwork();
    
    // Default JSON template
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
    this.useJsonMode = false;
    this.newAddon = { 
      name: '', 
      description: '', 
      services: [],
      volumes: [],
      networks: []
    };
    this.currentService = this.getEmptyService();
    this.currentVolume = this.getEmptyVolume();
    this.currentNetwork = this.getEmptyNetwork();
    this.addonJson = '';
    this.showServiceForm = false;
    this.showVolumeForm = false;
    this.showNetworkForm = false;
    this.editingServiceIndex = -1;
    this.editingVolumeIndex = -1;
    this.editingNetworkIndex = -1;
  }

  submitAddon(): void {
    try {
      let addonData: any;
      
      if (this.useJsonMode) {
        // JSON mode - parse entire addon object
        addonData = JSON.parse(this.addonJson);
      } else {
        // Manual mode - validate required fields
        if (!this.newAddon.name) {
          this.notificationService.error('Addon name is required');
          return;
        }
        if (!this.newAddon.description) {
          this.notificationService.error('Addon description is required');
          return;
        }
        if (this.newAddon.services.length === 0) {
          this.notificationService.error('At least one service is required');
          return;
        }
        
        // Validate each service has required fields
        for (const service of this.newAddon.services) {
          if (!service.service_name) {
            this.notificationService.error('All services must have a service_name');
            return;
          }
          if (!service.image) {
            this.notificationService.error('All services must have an image');
            return;
          }
        }
        
        addonData = this.newAddon;
      }
      
      // Validate addon data structure
      if (!addonData.name || !addonData.services || addonData.services.length === 0) {
        this.notificationService.error('Addon must have a name and at least one service');
        return;
      }
      
      this.marketplaceService.createAddon(addonData).subscribe({
        next: () => {
          this.notificationService.success('Addon submitted successfully!');
          this.notificationService.info('Verifying Docker images... Refresh in a few seconds to see it appear.');
          this.cancelForm();
        },
        error: (err) => this.notificationService.error(`Error: ${err.message}`)
      });
    } catch (e: any) {
      this.notificationService.error(`Invalid JSON: ${e.message}`);
    }
  }

  // ==== SERVICE MANAGEMENT ====
  addPortToCurrentService(item: { key: string, value: string }): void {
    this.currentService.ports = this.currentService.ports || {};
    this.currentService.ports[item.key] = item.value;
  }

  removePortFromCurrentService(key: string): void {
    if (this.currentService.ports) {
      delete this.currentService.ports[key];
    }
  }

  addEnvToCurrentService(item: { key: string, value: string }): void {
    this.currentService.environment = this.currentService.environment || {};
    this.currentService.environment[item.key] = item.value;
  }

  removeEnvFromCurrentService(key: string): void {
    if (this.currentService.environment) {
      delete this.currentService.environment[key];
    }
  }

  addVolumeMountToCurrentService(value: string): void {
    this.currentService.volumes = this.currentService.volumes || [];
    this.currentService.volumes.push(value);
    
    // Auto-create volume definition if it references a named volume
    const volumeName = value.split(':')[0];
    // Check if it's a named volume (not an absolute path like /host/path)
    if (!volumeName.startsWith('/') && !volumeName.startsWith('.')) {
      this.autoCreateVolume(volumeName);
    }
  }

  private autoCreateVolume(volumeName: string): void {
    this.newAddon.volumes = this.newAddon.volumes || [];
    const exists = this.newAddon.volumes.some(v => v.name === volumeName);
    
    if (!exists) {
      this.newAddon.volumes.push({
        name: volumeName,
        driver: 'local',
        driver_opts: {},
        labels: {}
      });
      this.notificationService.info(`Volume '${volumeName}' auto-created`);
    }
  }

  removeVolumeMountFromCurrentService(index: number): void {
    if (this.currentService.volumes) {
      this.currentService.volumes.splice(index, 1);
    }
  }

  addNetworkToCurrentService(value: string): void {
    this.currentService.networks = this.currentService.networks || [];
    this.currentService.networks.push(value);
    
    // Auto-create network definition
    this.autoCreateNetwork(value);
  }

  private autoCreateNetwork(networkName: string): void {
    this.newAddon.networks = this.newAddon.networks || [];
    const exists = this.newAddon.networks.some(n => n.name === networkName);
    
    if (!exists) {
      this.newAddon.networks.push({
        name: networkName,
        driver: 'bridge',
        enable_ipv6: false
      });
      this.notificationService.info(`Network '${networkName}' auto-created`);
    }
  }

  removeNetworkFromCurrentService(index: number): void {
    if (this.currentService.networks) {
      this.currentService.networks.splice(index, 1);
    }
  }

  addLabelToCurrentService(item: { key: string, value: string }): void {
    this.currentService.labels = this.currentService.labels || {};
    this.currentService.labels[item.key] = item.value;
  }

  removeLabelFromCurrentService(key: string): void {
    if (this.currentService.labels) {
      delete this.currentService.labels[key];
    }
  }

  async addServiceToAddon(): Promise<void> {
    if (!this.currentService.service_name || !this.currentService.image) {
      this.notificationService.error('Service name and image are required');
      return;
    }
    
    if (this.editingServiceIndex >= 0) {
      // Update existing service - ask for confirmation
      const confirmed = await this.confirmationService.confirm({
        title: 'Update Service',
        message: `Update the service '${this.newAddon.services[this.editingServiceIndex].service_name}'?`,
        confirmText: 'Update',
        cancelText: 'Cancel'
      });

      if (!confirmed) {
        return;
      }

      this.newAddon.services[this.editingServiceIndex] = { ...this.currentService };
      this.notificationService.success('Service updated!');
      this.editingServiceIndex = -1;
    } else {
      // Add new service
      this.newAddon.services.push({ ...this.currentService });
      this.notificationService.success('Service added!');
    }
    
    this.currentService = this.getEmptyService();
    this.showServiceForm = false;
  }

  editServiceFromAddon(index: number): void {
    this.currentService = { ...this.newAddon.services[index] };
    this.editingServiceIndex = index;
    this.showServiceForm = false; // Hide add form when editing
  }

  cancelServiceEdit(): void {
    this.currentService = this.getEmptyService();
    this.editingServiceIndex = -1;
  }

  showAddServiceForm(): void {
    this.cancelServiceEdit(); // Cancel any ongoing edits
    this.showServiceForm = true;
  }

  hideAddServiceForm(): void {
    this.currentService = this.getEmptyService();
    this.showServiceForm = false;
  }

  async removeServiceFromAddon(index: number): Promise<void> {
    const service = this.newAddon.services[index];
    const confirmed = await this.confirmationService.confirm({
      title: 'Delete Service',
      message: `Are you sure you want to delete the service '${service.service_name}'?`,
      confirmText: 'Delete',
      cancelText: 'Cancel'
    });

    if (confirmed) {
      this.newAddon.services.splice(index, 1);
      // Reset edit state if we're editing this service
      if (this.editingServiceIndex === index) {
        this.cancelServiceEdit();
      } else if (this.editingServiceIndex > index) {
        this.editingServiceIndex--;
      }
      this.notificationService.success('Service deleted');
    }
  }

  // ==== VOLUME MANAGEMENT ====
  addDriverOptToCurrentVolume(item: { key: string, value: string }): void {
    this.currentVolume.driver_opts = this.currentVolume.driver_opts || {};
    this.currentVolume.driver_opts[item.key] = item.value;
  }

  removeDriverOptFromCurrentVolume(key: string): void {
    if (this.currentVolume.driver_opts) {
      delete this.currentVolume.driver_opts[key];
    }
  }

  async addVolumeToAddon(): Promise<void> {
    if (!this.currentVolume.name) {
      this.notificationService.error('Volume name is required');
      return;
    }
    
    this.newAddon.volumes = this.newAddon.volumes || [];
    
    if (this.editingVolumeIndex >= 0) {
      // Update existing volume - ask for confirmation
      const confirmed = await this.confirmationService.confirm({
        title: 'Update Volume',
        message: `Update the volume '${this.newAddon.volumes[this.editingVolumeIndex].name}'?`,
        confirmText: 'Update',
        cancelText: 'Cancel'
      });

      if (!confirmed) {
        return;
      }

      this.newAddon.volumes[this.editingVolumeIndex] = { ...this.currentVolume };
      this.notificationService.success('Volume updated!');
      this.editingVolumeIndex = -1;
    } else {
      // Add new volume
      this.newAddon.volumes.push({ ...this.currentVolume });
      this.notificationService.success('Volume added!');
    }
    
    this.currentVolume = this.getEmptyVolume();
    this.showVolumeForm = false;
  }

  editVolumeFromAddon(index: number): void {
    this.currentVolume = { ...this.newAddon.volumes![index] };
    this.editingVolumeIndex = index;
    this.showVolumeForm = false; // Hide add form when editing
  }

  cancelVolumeEdit(): void {
    this.currentVolume = this.getEmptyVolume();
    this.editingVolumeIndex = -1;
  }

  showAddVolumeForm(): void {
    this.cancelVolumeEdit(); // Cancel any ongoing edits
    this.showVolumeForm = true;
  }

  hideAddVolumeForm(): void {
    this.currentVolume = this.getEmptyVolume();
    this.showVolumeForm = false;
  }

  async removeVolumeFromAddon(index: number): Promise<void> {
    if (this.newAddon.volumes) {
      const volume = this.newAddon.volumes[index];
      const confirmed = await this.confirmationService.confirm({
        title: 'Delete Volume',
        message: `Are you sure you want to delete the volume '${volume.name}'?`,
        confirmText: 'Delete',
        cancelText: 'Cancel'
      });

      if (confirmed) {
        this.newAddon.volumes.splice(index, 1);
        // Reset edit state if we're editing this volume
        if (this.editingVolumeIndex === index) {
          this.cancelVolumeEdit();
        } else if (this.editingVolumeIndex > index) {
          this.editingVolumeIndex--;
        }
        this.notificationService.success('Volume deleted');
      }
    }
  }

  // ==== NETWORK MANAGEMENT ====
  async addNetworkToAddon(): Promise<void> {
    if (!this.currentNetwork.name) {
      this.notificationService.error('Network name is required');
      return;
    }
    
    this.newAddon.networks = this.newAddon.networks || [];
    
    if (this.editingNetworkIndex >= 0) {
      // Update existing network - ask for confirmation
      const confirmed = await this.confirmationService.confirm({
        title: 'Update Network',
        message: `Update the network '${this.newAddon.networks[this.editingNetworkIndex].name}'?`,
        confirmText: 'Update',
        cancelText: 'Cancel'
      });

      if (!confirmed) {
        return;
      }

      this.newAddon.networks[this.editingNetworkIndex] = { ...this.currentNetwork };
      this.notificationService.success('Network updated!');
      this.editingNetworkIndex = -1;
    } else {
      // Add new network
      this.newAddon.networks.push({ ...this.currentNetwork });
      this.notificationService.success('Network added!');
    }
    
    this.currentNetwork = this.getEmptyNetwork();
    this.showNetworkForm = false;
  }

  editNetworkFromAddon(index: number): void {
    this.currentNetwork = { ...this.newAddon.networks![index] };
    this.editingNetworkIndex = index;
    this.showNetworkForm = false; // Hide add form when editing
  }

  cancelNetworkEdit(): void {
    this.currentNetwork = this.getEmptyNetwork();
    this.editingNetworkIndex = -1;
  }

  showAddNetworkForm(): void {
    this.cancelNetworkEdit(); // Cancel any ongoing edits
    this.showNetworkForm = true;
  }

  hideAddNetworkForm(): void {
    this.currentNetwork = this.getEmptyNetwork();
    this.showNetworkForm = false;
  }

  async removeNetworkFromAddon(index: number): Promise<void> {
    if (this.newAddon.networks) {
      const network = this.newAddon.networks[index];
      const confirmed = await this.confirmationService.confirm({
        title: 'Delete Network',
        message: `Are you sure you want to delete the network '${network.name}'?`,
        confirmText: 'Delete',
        cancelText: 'Cancel'
      });

      if (confirmed) {
        this.newAddon.networks.splice(index, 1);
        // Reset edit state if we're editing this network
        if (this.editingNetworkIndex === index) {
          this.cancelNetworkEdit();
        } else if (this.editingNetworkIndex > index) {
          this.editingNetworkIndex--;
        }
        this.notificationService.success('Network deleted');
      }
    }
  }

  // Helper methods for template
  getObjectKeys(obj: any): string[] {
    return obj ? Object.keys(obj) : [];
  }

  getObjectAsArray(obj: any): { key: string, value: string }[] {
    if (!obj) return [];
    return Object.keys(obj).map(key => ({ key, value: obj[key] }));
  }

  formatPorts(ports: { [key: string]: string } | undefined): string {
    if (!ports) return '';
    return Object.keys(ports).map(k => `${k}:${ports[k]}`).join(', ');
  }

  formatDriverOpts(opts: { [key: string]: string } | undefined): string {
    if (!opts) return '';
    return Object.keys(opts).map(k => `${k}=${opts[k]}`).join(', ');
  }

  async deleteAddon(id: string): Promise<void> {
    const confirmed = await this.confirmationService.confirm({
      title: 'Delete Addon',
      message: 'Are you sure you want to delete this addon from the marketplace?',
      confirmText: 'Delete',
      cancelText: 'Cancel'
    });

    if (!confirmed) {
      return;
    }

    this.marketplaceService.deleteAddon(id).subscribe({
      next: () => {
        this.notificationService.success('Addon deleted successfully!');
        this.loadAddons();
      },
      error: (err) => this.notificationService.error(`Error: ${err.message}`)
    });
  }

  installAddon(id: string): void {
    this.addonsEngineService.installAddon(id).subscribe({
      next: () => {
        this.notificationService.success('Addon installation started!');
      },
      error: (err) => this.notificationService.error(`Error: ${err.message}`)
    });
  }

  viewDetails(addon: MarketplaceAddon): void {
    this.selectedAddon = addon;
  }

  closeDetails(): void {
    this.selectedAddon = null;
  }

  getFormattedAddonJson(addon: MarketplaceAddon): string {
    const formatted = {
      name: addon.name,
      description: addon.description,
      services: addon.services?.map(service => ({
        service_name: service.service_name,
        image: service.image,
        command: service.command,
        ports: service.ports || {},
        environment: service.environment || {},
        volumes: service.volumes || [],
        networks: service.networks || [],
        labels: service.labels || {}
      })) || [],
      volumes: addon.volumes?.map(volume => ({
        name: volume.name,
        driver: volume.driver,
        driver_opts: volume.driver_opts || {},
        labels: volume.labels || {}
      })) || [],
      networks: addon.networks?.map(network => ({
        name: network.name,
        driver: network.driver,
        enable_ipv6: network.enable_ipv6 || false
      })) || []
    };
    return JSON.stringify(formatted, null, 2);
  }
}

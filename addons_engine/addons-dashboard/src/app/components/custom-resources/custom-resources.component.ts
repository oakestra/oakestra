import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ResourceAbstractorService } from '../../services/resource-abstractor.service';
import { NotificationService } from '../../services/notification.service';
import { ConfirmationService } from '../../services/confirmation.service';
import { CustomResource } from '../../models/addon.model';
import { RefreshButtonComponent } from '../shared/refresh-button/refresh-button.component';

type ViewMode = 'definitions' | 'instances';
type FormMode = 'create' | 'edit' | null;

@Component({
  selector: 'app-custom-resources',
  standalone: true,
  imports: [CommonModule, FormsModule, RefreshButtonComponent],
  templateUrl: './custom-resources.component.html',
  styleUrls: ['./custom-resources.component.css']
})
export class CustomResourcesComponent implements OnInit {
  // Expose Object to template
  Object = Object;
  // View control
  viewMode: ViewMode = 'definitions';

  // Resource Definitions
  definitions: CustomResource[] = [];
  definitionsLoading = false;
  definitionsError: string | null = null;
  showDefinitionForm: FormMode = null;
  selectedDefinition: CustomResource | null = null;
  newDefinition: CustomResource = { resource_type: '', schema: {} };
  schemaJson = '';

  // Resource Instances
  selectedResourceType: string = '';
  instances: any[] = [];
  instancesLoading = false;
  instancesError: string | null = null;
  showInstanceForm: FormMode = null;
  selectedInstance: any = null;
  newInstance: any = {};
  instanceJson = '';
  editingInstanceId: string = '';
  
  // Filtering
  showFilters = false;
  filterField: string = '';
  filterValue: string = '';
  activeFilters: { [key: string]: string } = {};

  constructor(
    private resourceAbstractorService: ResourceAbstractorService,
    private notificationService: NotificationService,
    private confirmationService: ConfirmationService
  ) {}

  ngOnInit(): void {
    this.loadDefinitions();
  }

  // View mode switching
  switchView(mode: ViewMode): void {
    this.viewMode = mode;
    if (mode === 'definitions') {
      this.loadDefinitions();
    } else if (mode === 'instances' && this.selectedResourceType) {
      this.loadInstances(this.selectedResourceType);
    }
  }

  // ===== RESOURCE DEFINITIONS CRUD =====

  loadDefinitions(): void {
    this.definitionsLoading = true;
    this.definitionsError = null;
    this.resourceAbstractorService.getCustomResources().subscribe({
      next: (data) => {
        this.definitions = data;
        this.definitionsLoading = false;
        // Auto-select first resource type if available
        if (this.definitions.length > 0 && !this.selectedResourceType) {
          this.selectedResourceType = this.definitions[0].resource_type;
        }
      },
      error: (err) => {
        this.definitionsError = `Failed to load definitions: ${err.message}`;
        this.definitionsLoading = false;
      }
    });
  }

  showCreateDefinitionForm(): void {
    this.showDefinitionForm = 'create';
    this.newDefinition = { resource_type: '', schema: {} };
    this.schemaJson = '{\n  "type": "object",\n  "properties": {\n    "name": {\n      "type": "string"\n    }\n  },\n  "required": ["name"]\n}';
  }

  cancelDefinitionForm(): void {
    this.showDefinitionForm = null;
    this.newDefinition = { resource_type: '', schema: {} };
    this.schemaJson = '';
  }

  submitDefinition(): void {
    try {
      this.newDefinition.schema = JSON.parse(this.schemaJson);
      this.resourceAbstractorService.createCustomResource(this.newDefinition).subscribe({
        next: () => {
          this.notificationService.success('Resource definition created successfully!');
          this.cancelDefinitionForm();
          this.loadDefinitions();
        },
        error: (err) => this.notificationService.error(`Error: ${err.message}`)
      });
    } catch (e: any) {
      this.notificationService.error(`Invalid JSON: ${e.message}`);
    }
  }

  viewDefinitionDetails(definition: CustomResource): void {
    this.selectedDefinition = definition;
  }

  closeDefinitionDetails(): void {
    this.selectedDefinition = null;
  }

  async deleteDefinition(definition: CustomResource): Promise<void> {
    const confirmed = await this.confirmationService.confirm({
      title: 'Delete Resource Definition',
      message: `Are you sure you want to delete the '${definition.resource_type}' resource definition?\n\n⚠️ WARNING: This will also delete ALL instances of this resource type!`,
      confirmText: 'Delete All',
      cancelText: 'Cancel'
    });

    if (!confirmed) {
      return;
    }

    this.resourceAbstractorService.deleteCustomResource(definition.resource_type).subscribe({
      next: () => {
        this.notificationService.success('Resource definition and all instances deleted successfully!');
        this.loadDefinitions();
        // Clear selected resource type if it was deleted
        if (this.selectedResourceType === definition.resource_type) {
          this.selectedResourceType = '';
          this.instances = [];
        }
      },
      error: (err) => this.notificationService.error(`Error: ${err.message}`)
    });
  }

  // ===== RESOURCE INSTANCES CRUD =====

  selectResourceType(resourceType: string): void {
    this.selectedResourceType = resourceType;
    this.loadInstances(resourceType);
  }

  loadInstances(resourceType: string): void {
    this.instancesLoading = true;
    this.instancesError = null;
    this.resourceAbstractorService.getResourcesByType(resourceType, this.activeFilters).subscribe({
      next: (data) => {
        this.instances = data;
        this.instancesLoading = false;
      },
      error: (err) => {
        this.instancesError = `Failed to load instances: ${err.message}`;
        this.instancesLoading = false;
      }
    });
  }

  showCreateInstanceForm(): void {
    this.showInstanceForm = 'create';
    this.instanceJson = '{\n  "name": "my-resource"\n}';
    this.newInstance = {};
  }

  showEditInstanceForm(instance: any): void {
    this.showInstanceForm = 'edit';
    this.editingInstanceId = instance._id;
    this.instanceJson = JSON.stringify(instance, null, 2);
  }

  cancelInstanceForm(): void {
    this.showInstanceForm = null;
    this.instanceJson = '';
    this.newInstance = {};
    this.editingInstanceId = '';
  }

  submitInstance(): void {
    try {
      const data = JSON.parse(this.instanceJson);
      
      if (this.showInstanceForm === 'create') {
        this.resourceAbstractorService.createResourceInstance(this.selectedResourceType, data).subscribe({
          next: () => {
            this.notificationService.success('Resource instance created successfully!');
            this.cancelInstanceForm();
            this.loadInstances(this.selectedResourceType);
          },
          error: (err) => this.notificationService.error(`Error: ${err.message}`)
        });
      } else if (this.showInstanceForm === 'edit') {
        // Remove _id from the data payload for updates
        const { _id, ...updateData } = data;
        this.resourceAbstractorService.updateResourceInstance(
          this.selectedResourceType, 
          this.editingInstanceId, 
          updateData
        ).subscribe({
          next: () => {
            this.notificationService.success('Resource instance updated successfully!');
            this.cancelInstanceForm();
            this.loadInstances(this.selectedResourceType);
          },
          error: (err) => this.notificationService.error(`Error: ${err.message}`)
        });
      }
    } catch (e: any) {
      this.notificationService.error(`Invalid JSON: ${e.message}`);
    }
  }

  viewInstanceDetails(instance: any): void {
    this.selectedInstance = instance;
  }

  closeInstanceDetails(): void {
    this.selectedInstance = null;
  }

  async deleteInstance(instance: any): Promise<void> {
    const confirmed = await this.confirmationService.confirm({
      title: 'Delete Resource Instance',
      message: `Are you sure you want to delete this ${this.selectedResourceType} instance?\\n\\nThis action cannot be undone.`,
      confirmText: 'Delete',
      cancelText: 'Cancel'
    });

    if (!confirmed) {
      return;
    }

    this.resourceAbstractorService.deleteResourceInstance(
      this.selectedResourceType, 
      instance._id
    ).subscribe({
      next: () => {
        this.notificationService.success('Resource instance deleted successfully!');
        this.loadInstances(this.selectedResourceType);
      },
      error: (err) => this.notificationService.error(`Error: ${err.message}`)
    });
  }

  // Helper methods for template
  getInstanceKeys(instance: any): string[] {
    return Object.keys(instance).filter(key => key !== '_id');
  }

  formatValue(value: any): string {
    if (value === null || value === undefined) return 'N/A';
    if (typeof value === 'object') return JSON.stringify(value);
    if (typeof value === 'string' && value.length > 50) return value.substring(0, 50) + '...';
    return String(value);
  }

  // Filter methods
  toggleFilters(): void {
    this.showFilters = !this.showFilters;
  }

  hasActiveFilters(): boolean {
    return Object.keys(this.activeFilters).length > 0;
  }

  getActiveFilterKeys(): string[] {
    return Object.keys(this.activeFilters);
  }

  addFilter(): void {
    if (this.filterField && this.filterValue) {
      this.activeFilters[this.filterField] = this.filterValue;
      this.filterField = '';
      this.filterValue = '';
      this.applyFilters();
    }
  }

  removeFilter(key: string): void {
    delete this.activeFilters[key];
    this.applyFilters();
  }

  clearFilters(): void {
    this.activeFilters = {};
    this.applyFilters();
  }

  applyFilters(): void {
    if (this.selectedResourceType) {
      this.loadInstances(this.selectedResourceType);
    }
  }
}

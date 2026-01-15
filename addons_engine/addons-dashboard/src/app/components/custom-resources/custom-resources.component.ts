import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ResourceAbstractorService } from '../../services/resource-abstractor.service';
import { CustomResource } from '../../models/addon.model';

@Component({
  selector: 'app-custom-resources',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './custom-resources.component.html',
  styleUrls: ['./custom-resources.component.css']
})
export class CustomResourcesComponent implements OnInit {
  resources: CustomResource[] = [];
  loading = false;
  error: string | null = null;
  showForm = false;
  selectedResource: CustomResource | null = null;

  newResource: CustomResource = {
    resource_type: '',
    schema: {}
  };
  schemaJson = '';

  constructor(private resourceAbstractorService: ResourceAbstractorService) {}

  ngOnInit(): void {
    this.loadResources();
  }

  loadResources(): void {
    this.loading = true;
    this.error = null;
    this.resourceAbstractorService.getCustomResources().subscribe({
      next: (data) => {
        this.resources = data;
        this.loading = false;
      },
      error: (err) => {
        this.error = `Failed to load custom resources: ${err.message}`;
        this.loading = false;
      }
    });
  }

  showAddForm(): void {
    this.showForm = true;
    this.schemaJson = '{\n  "type": "object",\n  "properties": {\n    "name": {\n      "type": "string"\n    }\n  },\n  "required": ["name"]\n}';
  }

  cancelForm(): void {
    this.showForm = false;
    this.newResource = { resource_type: '', schema: {} };
    this.schemaJson = '';
  }

  submitResource(): void {
    try {
      this.newResource.schema = JSON.parse(this.schemaJson);
      this.resourceAbstractorService.createCustomResource(this.newResource).subscribe({
        next: () => {
          alert('✅ Custom resource added successfully!');
          this.cancelForm();
          this.loadResources();
        },
        error: (err) => alert(`❌ Error: ${err.message}`)
      });
    } catch (e: any) {
      alert(`❌ Invalid JSON: ${e.message}`);
    }
  }

  viewDetails(resource: CustomResource): void {
    this.selectedResource = resource;
  }

  closeDetails(): void {
    this.selectedResource = null;
  }
}

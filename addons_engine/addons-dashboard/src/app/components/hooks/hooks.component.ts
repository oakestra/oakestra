import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ResourceAbstractorService } from '../../services/resource-abstractor.service';
import { Hook, HookEvent } from '../../models/addon.model';

@Component({
  selector: 'app-hooks',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './hooks.component.html',
  styleUrls: ['./hooks.component.css']
})
export class HooksComponent implements OnInit {
  hooks: Hook[] = [];
  loading = false;
  error: string | null = null;
  showForm = false;
  selectedHook: Hook | null = null;

  newHook: Hook = {
    hook_name: '',
    webhook_url: '',
    entity: '',
    events: []
  };

  availableEvents = Object.values(HookEvent);
  selectedEvents: { [key: string]: boolean } = {};

  constructor(private resourceAbstractorService: ResourceAbstractorService) {}

  ngOnInit(): void {
    this.loadHooks();
  }

  loadHooks(): void {
    this.loading = true;
    this.error = null;
    this.resourceAbstractorService.getHooks().subscribe({
      next: (data) => {
        this.hooks = data;
        this.loading = false;
      },
      error: (err) => {
        this.error = `Failed to load hooks: ${err.message}`;
        this.loading = false;
      }
    });
  }

  showAddForm(): void {
    this.showForm = true;
    this.selectedEvents = {};
  }

  cancelForm(): void {
    this.showForm = false;
    this.newHook = { hook_name: '', webhook_url: '', entity: '', events: [] };
    this.selectedEvents = {};
  }

  submitHook(): void {
    this.newHook.events = Object.keys(this.selectedEvents)
      .filter(key => this.selectedEvents[key])
      .map(key => key as HookEvent);

    this.resourceAbstractorService.createHook(this.newHook).subscribe({
      next: () => {
        alert('✅ Hook added successfully!');
        this.cancelForm();
        this.loadHooks();
      },
      error: (err) => alert(`❌ Error: ${err.message}`)
    });
  }

  deleteHook(id: string): void {
    if (!confirm('Are you sure you want to delete this hook?')) {
      return;
    }
    this.resourceAbstractorService.deleteHook(id).subscribe({
      next: () => {
        alert('✅ Hook deleted successfully!');
        this.loadHooks();
      },
      error: (err) => alert(`❌ Error: ${err.message}`)
    });
  }

  viewDetails(hook: Hook): void {
    this.selectedHook = hook;
  }

  closeDetails(): void {
    this.selectedHook = null;
  }
}

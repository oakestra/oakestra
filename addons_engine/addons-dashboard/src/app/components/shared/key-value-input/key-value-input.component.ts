import { Component, Input, Output, EventEmitter } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

@Component({
  selector: 'app-key-value-input',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './key-value-input.component.html',
  styleUrls: ['./key-value-input.component.scss']
})
export class KeyValueInputComponent {
  @Input() items: { key: string, value: string }[] = [];
  @Input() separator: string = '=';
  @Input() keyPlaceholder: string = 'Key';
  @Input() valuePlaceholder: string = 'Value';
  @Input() addLabel: string = 'Add';
  @Input() emptyMessage: string = 'No items added yet';
  @Output() add = new EventEmitter<{ key: string, value: string }>();
  @Output() remove = new EventEmitter<string>();

  newKey: string = '';
  newValue: string = '';

  onAdd(): void {
    if (this.newKey.trim() && this.newValue.trim()) {
      this.add.emit({ key: this.newKey.trim(), value: this.newValue.trim() });
      this.newKey = '';
      this.newValue = '';
    }
  }

  onRemove(key: string): void {
    this.remove.emit(key);
  }

  onKeyPress(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      event.preventDefault();
      this.onAdd();
    }
  }
}

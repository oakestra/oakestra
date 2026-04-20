import { Component, Input, Output, EventEmitter } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

@Component({
  selector: 'app-simple-list-input',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './simple-list-input.component.html',
  styleUrls: ['./simple-list-input.component.scss']
})
export class SimpleListInputComponent {
  @Input() items: string[] = [];
  @Input() placeholder: string = 'Enter value';
  @Input() addLabel: string = 'Add';
  @Input() emptyMessage: string = 'No items added yet';
  @Output() add = new EventEmitter<string>();
  @Output() remove = new EventEmitter<number>();

  newValue: string = '';

  onAdd(): void {
    if (this.newValue.trim()) {
      this.add.emit(this.newValue.trim());
      this.newValue = '';
    }
  }

  onRemove(index: number): void {
    this.remove.emit(index);
  }

  onKeyPress(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      event.preventDefault();
      this.onAdd();
    }
  }
}

import { Component, Output, EventEmitter } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-refresh-button',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './refresh-button.component.html',
  styleUrls: ['./refresh-button.component.scss']
})
export class RefreshButtonComponent {
  @Output() refresh = new EventEmitter<void>();

  isRefreshing = false;

  onRefresh(): void {
    if (this.isRefreshing) return;

    this.isRefreshing = true;
    this.refresh.emit();

    // Reset animation after it completes
    setTimeout(() => {
      this.isRefreshing = false;
    }, 600);
  }
}

import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ConfirmationService, ConfirmationDialog } from '../../services/confirmation.service';

@Component({
  selector: 'app-confirmation-dialog',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './confirmation-dialog.component.html',
  styleUrls: ['./confirmation-dialog.component.scss']
})
export class ConfirmationDialogComponent implements OnInit {
  currentDialog: ConfirmationDialog | null = null;
  private currentCallback: ((result: boolean) => void) | null = null;

  constructor(private confirmationService: ConfirmationService) {}

  ngOnInit(): void {
    this.confirmationService.confirmation$.subscribe(({ dialog, callback }) => {
      this.currentDialog = dialog;
      this.currentCallback = callback;
    });
  }

  formatMessage(message: string): string {
    return message.replace(/\n/g, '<br>');
  }

  confirm(): void {
    if (this.currentCallback) {
      this.currentCallback(true);
    }
    this.close();
  }

  cancel(): void {
    if (this.currentCallback) {
      this.currentCallback(false);
    }
    this.close();
  }

  private close(): void {
    this.currentDialog = null;
    this.currentCallback = null;
  }
}

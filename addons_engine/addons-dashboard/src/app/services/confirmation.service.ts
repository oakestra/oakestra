import { Injectable } from '@angular/core';
import { Subject, Observable } from 'rxjs';

export interface ConfirmationDialog {
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
}

@Injectable({
  providedIn: 'root'
})
export class ConfirmationService {
  private confirmationSubject = new Subject<{ dialog: ConfirmationDialog; callback: (result: boolean) => void }>();
  
  confirmation$ = this.confirmationSubject.asObservable();

  confirm(dialog: ConfirmationDialog): Promise<boolean> {
    return new Promise((resolve) => {
      this.confirmationSubject.next({
        dialog,
        callback: resolve
      });
    });
  }
}

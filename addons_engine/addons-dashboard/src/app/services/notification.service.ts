import { Injectable } from '@angular/core';
import { NotificationType } from '../models/notification';

@Injectable({
  providedIn: 'root'
})
export class NotificationService {
  private notifications: Array<{ message: string; type: NotificationType; id: number }> = [];
  private nextId = 0;

  getNotifications() {
    return this.notifications;
  }

  notify(type: NotificationType, message: string): void {
    const id = this.nextId++;
    this.notifications.push({ message, type, id });

    // Auto-remove after 3 seconds
    setTimeout(() => {
      this.remove(id);
    }, 3000);
  }

  remove(id: number): void {
    this.notifications = this.notifications.filter(n => n.id !== id);
  }

  success(message: string): void {
    this.notify(NotificationType.success, message);
  }

  error(message: string): void {
    this.notify(NotificationType.error, message);
  }

  info(message: string): void {
    this.notify(NotificationType.info, message);
  }
}

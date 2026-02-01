import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';
import { NotificationType } from '../../models/notification';
import { NotificationService } from '../../services/notification.service';

@Component({
  selector: 'app-notification',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './notification.component.html',
  styleUrls: ['./notification.component.scss']
})
export class NotificationComponent {
  NotificationType = NotificationType;

  constructor(public notificationService: NotificationService) {}
}

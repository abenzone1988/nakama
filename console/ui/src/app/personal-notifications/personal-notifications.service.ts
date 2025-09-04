import { Injectable } from '@angular/core';
import {
  ConsoleService,
  ListPersonalNotificationLogResponse,
  CreatePersonalNotificationRequest
} from '../console.service';
import { Observable } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class PersonalNotificationsService {
  constructor(private readonly consoleService: ConsoleService) {}

  sendPersonalNotification(data: CreatePersonalNotificationRequest): Observable<any> {
    return this.consoleService.createPersonalNotification('', data);
  }

  getPersonalNotificationLogs(params: { limit?: number; cursor?: string; filter?: string }): Observable<ListPersonalNotificationLogResponse> {
    return this.consoleService.listPersonalNotificationLogs('', params.filter, '', '', params.cursor, params.limit);
  }
}

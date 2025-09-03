import { Injectable } from '@angular/core';
import { ConsoleService, ListPersonalNotificationLogResponse, CreateSystemNotificationRequest } from '../console.service';
import { Observable } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class PersonalNotificationsService {
  constructor(private readonly consoleService: ConsoleService) {}

  sendPersonalNotification(data: CreateSystemNotificationRequest): Observable<any> {
    return this.consoleService.createSystemNotification('', data);
  }

  getPersonalNotificationLogs(params: { limit?: number; cursor?: string; filter?: string }): Observable<ListPersonalNotificationLogResponse> {
    return this.consoleService.listPersonalNotificationLogs('', params.filter, '', '', params.cursor, params.limit);
  }
}

import { Injectable } from '@angular/core';
import { ConsoleService, SystemNotice, ListSystemNoticeResponse } from '../console.service';
import { Observable } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class SystemNotificationsService {
  constructor(private readonly consoleService: ConsoleService) {}

  getNotifications(params: { limit?: number; cursor?: string; filter?: string }): Observable<ListSystemNoticeResponse> {
    return this.consoleService.listSystemNotifications('', params.filter, params.cursor, params.limit);
  }

  createNotification(data: any): Observable<SystemNotice> {
    return this.consoleService.createSystemNotification('', data);
  }

  updateNotification(id: string, data: any): Observable<SystemNotice> {
    return this.consoleService.updateSystemNotification('', id, data);
  }

  deleteNotification(id: string): Observable<any> {
    return this.consoleService.deleteSystemNotification('', id);
  }
}

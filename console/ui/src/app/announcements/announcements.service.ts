import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { ConsoleService, Announcement, AnnouncementList } from '../console.service';

@Injectable({
  providedIn: 'root'
})
export class AnnouncementsService {
  constructor(private consoleService: ConsoleService) {}

  getAnnouncements(params: {
    limit?: number;
    cursor?: string;
    status?: number;
  }): Observable<AnnouncementList> {
    return this.consoleService.listAnnouncements(
      localStorage.getItem('token') || '',
      params.status,
      params.limit,
      params.cursor
    );
  }

  getAnnouncement(id: string): Observable<Announcement> {
    return this.consoleService.getAnnouncement(localStorage.getItem('token') || '', id);
  }

  createAnnouncement(data: Partial<Announcement>): Observable<Announcement> {
    return this.consoleService.createAnnouncement(localStorage.getItem('token') || '', {
      content: data.content || '',
      img: data.img || '',
      status: data.status || 0,
      title: data.title || ''
    });
  }

  updateAnnouncement(id: string, data: Partial<Announcement>): Observable<Announcement> {
    return this.consoleService.updateAnnouncement(localStorage.getItem('token') || '', id, {
      content: data.content || '',
      img: data.img || '',
      status: data.status || 0,
      title: data.title || ''
    });
  }

  deleteAnnouncement(id: string): Observable<void> {
    return this.consoleService.deleteAnnouncement(localStorage.getItem('token') || '', id);
  }
} 
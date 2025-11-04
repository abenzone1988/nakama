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
      '',
      params.status,
      params.limit,
      params.cursor
    );
  }

  getAnnouncement(id: string): Observable<Announcement> {
    return this.consoleService.getAnnouncement('', id);
  }

  createAnnouncement(data: Partial<Announcement>): Observable<Announcement> {
    return this.consoleService.createAnnouncement( '', {
      content: data.content || '',
      img: data.img || '',
      status: data.status || 0,
      title: data.title || ''
    });
  }

  updateAnnouncement(id: string, data: Partial<Announcement>): Observable<Announcement> {
    return this.consoleService.updateAnnouncement('', id, {
      content: data.content || '',
      img: data.img || '',
      status: data.status || 0,
      title: data.title || ''
    });
  }

  deleteAnnouncement(id: string): Observable<void> {
    return this.consoleService.deleteAnnouncement('', id);
  }

  searchAnnouncements(params: {
    query: string;
    limit?: number;
    cursor?: string;
  }): Observable<AnnouncementList> {
    return this.consoleService.searchAnnouncements(
      '',
      params.query,
      params.limit,
      params.cursor
    );
  }
}

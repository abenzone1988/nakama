// Copyright 2020 The Nakama Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import {Component, inject, Injectable, OnInit, TemplateRef} from '@angular/core';
import {ActivatedRoute, ActivatedRouteSnapshot, Resolve, Router, RouterStateSnapshot} from '@angular/router';
import {
  ConsoleService,
  SystemNotice,
  ListSystemNoticeResponse, CreateSystemNotificationRequest, GameItem, NoticeContent, UserRole,
} from '../console.service';
import {Observable} from 'rxjs';
import {FormBuilder, FormGroup, FormControl, FormArray, AbstractControl, Validators, ReactiveFormsModule} from '@angular/forms';
import {AuthenticationService} from '../authentication.service';
import {NgbModal, NgbCalendar, NgbDateStruct, NgbTimeStruct, NgbDate, NgbAlert, NgbModule} from '@ng-bootstrap/ng-bootstrap';
import {SystemNotificationsService} from './system-notifications.service';
import {DeleteConfirmService} from '../shared/delete-confirm.service';
import {CommonModule} from '@angular/common';

import {ModalDismissReasons} from '@ng-bootstrap/ng-bootstrap';

interface NotificationResponse {
  notifications?: SystemNotice[];
  total_count?: number;
  next_cursor?: string;
}

@Component({
  templateUrl: './system-notifications.component.html',
  styleUrls: ['./system-notifications.component.scss'],
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, NgbModule]
})
export class SystemNotificationsComponent implements OnInit {
  private today: NgbDate;
  private formItems: FormArray;
  searchForm: FormGroup;
  notificationForm: FormGroup;
  items: FormArray;

  constructor(
    private readonly route: ActivatedRoute,
    private readonly router: Router,
    private readonly consoleService: ConsoleService,
    private readonly authService: AuthenticationService,
    private readonly formBuilder: FormBuilder,
    private readonly notificationsService: SystemNotificationsService,
    private readonly modalService: NgbModal,
    private readonly deleteConfirmService: DeleteConfirmService,
    private readonly calendar: NgbCalendar,
  ) {
    this.today = this.calendar.getToday();
    this.formItems = this.formBuilder.array([]);
    this.items = this.formBuilder.array([]);
    this.searchForm = this.formBuilder.group({
      filter: [''],
      status: ['']
    });

    this.notificationForm = this.formBuilder.group({
      type: [0],
      subject: [''],
      desc: [''],
      targetIds: this.formBuilder.array([]),
      items: this.formBuilder.array([]),
      enableExpiry: [false],
      expireDate: [{value: null, disabled: true}],
      expireTime: [{value: null, disabled: true}]
    });

    this.notificationForm.get('enableExpiry')!.valueChanges.subscribe(enabled => {
      if (enabled) {
        this.notificationForm.get('expireDate')!.enable();
        this.notificationForm.get('expireTime')!.enable();
      } else {
        this.notificationForm.get('expireDate')!.disable();
        this.notificationForm.get('expireTime')!.disable();
      }
    });

    this.notificationForm.get('expireDate')!.disable();
    this.notificationForm.get('expireTime')!.disable();

    this.route.data.subscribe({
      next: (data) => {
        this.notifications.length = 0;
        if (data && data[0]) {
          const response = data[0];
          if (response.notice) {
            this.notifications.push(...response.notice);
          }
          this.notificationsCount = response.total_count || 0;
          this.nextCursor = response.cursor || '';
          this.prevCursor = response.prev_cursor || '';
        }
      },
      error: (err) => {
        console.error('Error loading notifications:', err);
        this.error = err;
      }
    });
  }

  get s(): any {
    return this.searchForm.controls;
  }

  get n(): any {
    return this.notificationForm.controls;
  }

  public readonly systemUserId = '00000000-0000-0000-0000-000000000000';
  public error = '';
  public notificationsCount = 0;
  public notifications: SystemNotice[] = [];
  public nextCursor = '';
  public prevCursor = '';
  closeResult = '';
  showSuccess = false;
  showError = false;
  errorMessage = '';
  loading = false;
  totalCount = 0;
  currentPage = 1;
  pageSize = 10;
  statusFilter = '';
  searchQuery = '';
  cursor = '';
  cursorCache: { [page: number]: string } = { 1: '' };
  isSearchMode = false;
  editingNotification: SystemNotice | null = null;

  defaultItems = [
    {id: '10000', name: '金币', icon: 'CoinPile'},
    {id: '10001', name: '宝石', icon: 'GemPile'},
    {id: '10002', name: '体力', icon: 'Stanima'},
    {id: '20000', name: '面广告券', icon: 'BigFood'},
  ];

  ngOnInit(): void {
    const qp = this.route.snapshot.queryParamMap;
    const filterControl = this.searchForm.get('filter');
    if (filterControl) {
      filterControl.setValue(qp.get('filter'));
    }
    this.nextCursor = qp.get('cursor') ?? '';

    if (this.nextCursor && this.nextCursor !== '') {
      this.search(1);
    } else if (filterControl?.value) {
      this.search(0);
    }

    this.loadNotifications();
  }

  loadNotifications(): void {
    this.loading = true;

    if (this.isSearchMode) {
      this.loadSearchResults();
    } else {
      this.loadNotificationsList();
    }
  }

  private loadSearchResults(): void {
    const params: any = {
      query: this.searchQuery,
      limit: this.pageSize,
      cursor: this.cursor,
    };

    this.fetchNotifications(params, true);
  }

  private loadNotificationsList(): void {
    const params: any = {
      limit: this.pageSize,
      cursor: this.cursor,
    };
    if (this.statusFilter !== '') {
      params.status = parseInt(this.statusFilter, 10);
    }

    this.fetchNotifications(params, false);
  }

  private fetchNotifications(params: any, isSearch: boolean): void {
    this.notificationsService.getNotifications(params).subscribe({
      next: (response: NotificationResponse) => {
        this.notifications = response.notifications || [];
        this.totalCount = response.total_count || 0;
        if (response.next_cursor) {
          this.cursorCache[this.currentPage + 1] = response.next_cursor;
        }
        this.loading = false;
      },
      error: (error) => {
        console.error(isSearch ? '搜索通知失败' : '加载通知列表失败', error);
        this.loading = false;
      }
    });
  }

  search(state: number): void {
    let cursor = '';
    switch (state) {
      case -1:
        cursor = this.prevCursor;
        break;
      case 0:
        cursor = '';
        break;
      case 1:
        cursor = this.nextCursor;
        break;
    }

    this.consoleService.listSystemNotifications('', this.s.filter.value, cursor).subscribe(d => {
      this.error = '';

      this.notifications.length = 0;
      if (d.notifications) {
        this.notifications.push(...d.notifications);
      }
      this.notificationsCount = d.total_count ?? 0;
      this.nextCursor = d.next_cursor ?? '';
      this.prevCursor = d.prev_cursor ?? '';

      this.router.navigate([], {
        relativeTo: this.route,
        queryParams: {
          limit: 50,
          cursor
        },
        queryParamsHandling: 'merge',
      });
    }, err => {
      this.error = err;
    });
  }

  private getDismissReason(reason: any): string {
    switch (reason) {
      case ModalDismissReasons.ESC:
        return 'by pressing ESC';
      case ModalDismissReasons.BACKDROP_CLICK:
        return 'by clicking on a backdrop';
      default:
        return `with: ${reason}`;
    }
  }

  private toTimeString(date: NgbDateStruct | null, time: NgbTimeStruct | null): string | undefined {
    if (!date) {
      return undefined;
    }
    const timeStr = time ? `${time.hour.toString().padStart(2, '0')}:${time.minute.toString().padStart(2, '0')}:00` : '00:00:00';
    return `${date.year}-${date.month.toString().padStart(2, '0')}-${date.day.toString().padStart(2, '0')}T${timeStr}Z`;
  }

  convertItemsToGameItems(): GameItem[] {
    return this.formItems.controls.map((control: AbstractControl) => {
      const group = control as FormGroup;
      return {
        id: group.get('id')?.value,
        num: group.get('num')?.value?.toString(),
      };
    });
  }

  addTargetId(): void {
    const targetIds = this.notificationForm.get('targetIds') as FormArray;
    targetIds.push(this.formBuilder.control(''));
  }

  removeTargetId(index: number): void {
    const targetIds = this.notificationForm.get('targetIds') as FormArray;
    targetIds.removeAt(index);
  }

  setToday(): void {
    this.notificationForm.patchValue({expireDate: this.calendar.getToday()});
  }

  addItem(): void {
    const items = this.notificationForm.get('items') as FormArray;
    items.push(this.formBuilder.group({
      id: [''],
      num: [1]
    }));
  }

  removeItem(index: number): void {
    const items = this.notificationForm.get('items') as FormArray;
    items.removeAt(index);
  }

  getIconById(itemId: string): string {
    const item = this.defaultItems.find(item => item.id === itemId);
    return item ? `/static/icon/${item.icon}.png` : '';
  }

  operateAllowed(): boolean{
    // only admin and developers are allowed.
    return this.authService.sessionRole <= UserRole.USER_ROLE_DEVELOPER;
  }

  openModal(content: any, notification?: SystemNotice): void {
    this.editingNotification = notification || null;
    if (notification) {
      this.notificationForm.patchValue({
        subject: notification.subject,
        content: notification.content || { description: '', rewards: [] },
      });
      this.formItems.clear();
      if (notification.content?.rewards) {
        notification.content.rewards.forEach(reward => {
          this.formItems.push(this.formBuilder.group({
            id: [reward.id],
            num: [parseInt(reward.num || '1', 10)],
          }));
        });
      }
    } else {
      this.notificationForm.reset({
        subject: '',
        content: { description: '', rewards: [] },
        code: 0,
        enableExpiry: false,
        expireDate: null,
        expireTime: null,
      });
      this.formItems.clear();
    }
    const modalRef = this.modalService.open(content, {
      size: 'lg',
      windowClass: 'notification-modal',
      backdropClass: 'notification-backdrop',
      modalDialogClass: 'notification-dialog'
    });

    setTimeout(() => {
      const backdrop = document.querySelector('.notification-backdrop') as HTMLElement;
      const modal = document.querySelector('.notification-modal') as HTMLElement;
      if (backdrop) {
        backdrop.style.zIndex = '1040';
      }
      if (modal) {
        modal.style.zIndex = '1050';
      }
    });
  }

  onSubmit(): void {
    if (this.notificationForm.invalid) {
      return;
    }
    const formValue = this.notificationForm.value;
    const notice: SystemNotice = {
      subject: formValue.subject,
      content: {
        description: formValue.desc,
        rewards: this.convertItemsToGameItems(),
      },
      expiry_time: formValue.enableExpiry ? this.toTimeString(formValue.expireDate, formValue.expireTime) : undefined
    };

    if (this.editingNotification?.id) {
      this.notificationsService.updateNotification(this.editingNotification.id, notice).subscribe({
        next: () => {
          this.showSuccess = true;
          this.showError = false;
          setTimeout(() => this.showSuccess = false, 3000);
          this.search(0);
          this.modalService.dismissAll();
          this.loadNotifications();
        },
        error: (error) => {
          this.showSuccess = false;
          this.showError = true;
          setTimeout(() => this.showError = false, 3000);
          this.search(0);
          console.error('更新失败', error);
        }
      });
    } else {
      const createSystemNotificationRequest: CreateSystemNotificationRequest = {
        type: formValue.type,
        target: formValue.targetIds,
        notice
      };
      this.notificationsService.createNotification(createSystemNotificationRequest).subscribe({
        next: () => {
          this.showSuccess = true;
          this.showError = false;
          setTimeout(() => this.showSuccess = false, 3000);
          this.search(0);
          this.modalService.dismissAll();
          this.loadNotifications();
        },
        error: (error) => {
          this.showSuccess = false;
          this.showError = true;
          setTimeout(() => this.showError = false, 3000);
          this.search(0);
          console.error('创建失败', error);
        }
      });
    }
  }

  deleteNotification(notification: SystemNotice): void {
    if (!notification.id) {
      return;
    }
    this.deleteConfirmService.openDeleteConfirmModal(
      () => {
        this.notificationsService.deleteNotification(notification.id!).subscribe({
          next: () => {
            this.loadNotifications();
          },
          error: (error) => {
            console.error('删除失败', error);
          }
        });
      },
      undefined,
      '删除通知',
      `确认删除标题为 "${notification.subject}" 的通知吗？`
    );
  }

  onSearch(): void {
    this.searchQuery = this.searchForm.get('filter')?.value || '';
    this.statusFilter = this.searchForm.get('status')?.value || '';
    this.isSearchMode = true;
    this.currentPage = 1;
    this.cursor = '';
    this.cursorCache = { 1: '' };
    this.loadNotifications();
  }

  clearSearch(): void {
    this.searchForm.reset();
    this.searchQuery = '';
    this.statusFilter = '';
    this.isSearchMode = false;
    this.currentPage = 1;
    this.cursor = '';
    this.cursorCache = { 1: '' };
    this.loadNotifications();
  }

  onStatusFilterChange(): void {
    this.isSearchMode = false;
    this.searchQuery = '';
    this.currentPage = 1;
    this.cursor = '';
    this.cursorCache = { 1: '' };
    this.loadNotifications();
  }

  onPageChange(page: number): void {
    if (page > this.currentPage) {
      this.cursor = this.cursorCache[page] || '';
    } else if (page === 1) {
      this.cursor = '';
    } else {
      this.cursor = this.cursorCache[page] || '';
    }
    this.currentPage = page;
    this.loadNotifications();
  }

  getStatusText(status: number): string {
    const statusMap: { [key: number]: string } = {
      0: '草稿',
      1: '已发布',
      2: '已下线'
    };
    return statusMap[status] || '未知';
  }

  getStatusClass(status: number): string {
    const classMap: { [key: number]: string } = {
      0: 'badge-secondary',
      1: 'badge-success',
      2: 'badge-danger'
    };
    return classMap[status] || 'badge-secondary';
  }

}

@Injectable({providedIn: 'root'})
export class MailManagerResolver implements Resolve<ListSystemNoticeResponse> {
  constructor(private readonly consoleService: ConsoleService) {
  }

  resolve(route: ActivatedRouteSnapshot, state: RouterStateSnapshot): Observable<ListSystemNoticeResponse> {
    const filter = route.queryParamMap.get('filter') || '';
    return this.consoleService.listSystemNotifications('', filter, undefined);
  }
}


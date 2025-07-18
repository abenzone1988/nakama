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
      challengeId: [''],
      targetIds: this.formBuilder.array([]),
      items: this.formBuilder.array([]),
      immediateSend: [true], // 默认选择立即发送
      enableEffectiveTime: [false], // 设置生效时间
      enableExpiry: [false], // 设置过期时间
      effectiveDate: [{value: null, disabled: true}],
      effectiveTime: [{value: null, disabled: true}],
      expireDate: [{value: null, disabled: true}],
      expireTime: [{value: null, disabled: true}]
    });

    this.notificationForm.get('enableEffectiveTime')!.valueChanges.subscribe(enabled => {
      if (enabled) {
        this.notificationForm.get('effectiveDate')!.enable();
        this.notificationForm.get('effectiveTime')!.enable();
      } else {
        this.notificationForm.get('effectiveDate')!.disable();
        this.notificationForm.get('effectiveTime')!.disable();
      }
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

    this.notificationForm.get('immediateSend')!.valueChanges.subscribe(immediate => {
      if (immediate) {
        this.notificationForm.get('enableEffectiveTime')!.disable();
        this.notificationForm.get('enableExpiry')!.disable();
        this.notificationForm.get('effectiveDate')!.disable();
        this.notificationForm.get('effectiveTime')!.disable();
        this.notificationForm.get('expireDate')!.disable();
        this.notificationForm.get('expireTime')!.disable();
      } else {
        this.notificationForm.get('enableEffectiveTime')!.enable();
        this.notificationForm.get('enableExpiry')!.enable();
        if (this.notificationForm.get('enableEffectiveTime')!.value) {
          this.notificationForm.get('effectiveDate')!.enable();
          this.notificationForm.get('effectiveTime')!.enable();
        }
        if (this.notificationForm.get('enableExpiry')!.value) {
          this.notificationForm.get('expireDate')!.enable();
          this.notificationForm.get('expireTime')!.enable();
        }
      }
    });

    this.notificationForm.get('effectiveDate')!.disable();
    this.notificationForm.get('effectiveTime')!.disable();
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

  challenges: any[] = [];
  selectedChallenge: any = null;

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
    this.loadChallenges();
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
    this.notificationForm.patchValue({effectiveDate: this.calendar.getToday()});
  }

  setExpireToday(): void {
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
      // 检查是否为立即发送（生效时间等于创建时间）
      const isImmediateSend = notification.effective_time && 
        new Date(notification.effective_time).getTime() === new Date(notification.create_time!).getTime();
      
      this.notificationForm.patchValue({
        subject: notification.subject,
        content: notification.content || { description: '', rewards: [] },
        immediateSend: isImmediateSend,
        enableExpiry: !isImmediateSend && (!!notification.effective_time || !!notification.expiry_time),
      });

      // 设置生效时间
      if (notification.effective_time && !isImmediateSend) {
        const effectiveDate = new Date(notification.effective_time);
        this.notificationForm.patchValue({
          effectiveDate: {
            year: effectiveDate.getFullYear(),
            month: effectiveDate.getMonth() + 1,
            day: effectiveDate.getDate()
          },
          effectiveTime: {
            hour: effectiveDate.getHours(),
            minute: effectiveDate.getMinutes()
          }
        });
      }

      // 设置过期时间
      if (notification.expiry_time) {
        const expireDate = new Date(notification.expiry_time);
        this.notificationForm.patchValue({
          expireDate: {
            year: expireDate.getFullYear(),
            month: expireDate.getMonth() + 1,
            day: expireDate.getDate()
          },
          expireTime: {
            hour: expireDate.getHours(),
            minute: expireDate.getMinutes()
          }
        });
      }
      this.formItems.clear();
      if (notification.content?.rewards) {
        notification.content.rewards.forEach(reward => {
          this.formItems.push(this.formBuilder.group({
            id: [reward.id],
            num: [parseInt(String(reward.num || '1'), 10)],
          }));
        });
      }
    } else {
      this.notificationForm.reset({
        subject: '',
        content: { description: '', rewards: [] },
        code: 0,
        immediateSend: true,
        enableEffectiveTime: false,
        enableExpiry: false,
        effectiveDate: null,
        effectiveTime: null,
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
    
    // 验证生效时间不能小于当前时间
    if (formValue.enableEffectiveTime && formValue.effectiveDate) {
      const effectiveDate = new Date(formValue.effectiveDate.year, formValue.effectiveDate.month - 1, formValue.effectiveDate.day);
      const now = new Date();
      if (effectiveDate < now) {
        this.showError = true;
        this.errorMessage = '生效时间不能小于当前时间';
        setTimeout(() => this.showError = false, 3000);
        return;
      }
    }

    // 验证过期时间不能小于生效时间
    if (formValue.enableExpiry && formValue.expireDate && formValue.effectiveDate && formValue.enableEffectiveTime) {
      const effectiveDate = new Date(formValue.effectiveDate.year, formValue.effectiveDate.month - 1, formValue.effectiveDate.day);
      const expireDate = new Date(formValue.expireDate.year, formValue.expireDate.month - 1, formValue.expireDate.day);
      if (expireDate <= effectiveDate) {
        this.showError = true;
        this.errorMessage = '过期时间必须大于生效时间';
        setTimeout(() => this.showError = false, 3000);
        return;
      }
    }

    // 处理立即发送逻辑
    let effectiveTime: string | undefined;
    if (formValue.immediateSend) {
      // 立即发送，使用当前时间作为生效时间
      const now = new Date();
      effectiveTime = now.toISOString();
    } else if (formValue.enableEffectiveTime && formValue.effectiveDate) {
      // 手动选择生效时间
      effectiveTime = this.toTimeString(formValue.effectiveDate, formValue.effectiveTime);
    } else {
      // 如果没有选择立即发送也没有设置生效时间，使用当前时间
      const now = new Date();
      effectiveTime = now.toISOString();
    }

    const notice: SystemNotice = {
      subject: formValue.subject,
      content: {
        description: formValue.desc,
        rewards: this.convertItemsToGameItems(),
      },
      effective_time: effectiveTime,
      expiry_time: formValue.enableExpiry && formValue.expireDate ? this.toTimeString(formValue.expireDate, formValue.expireTime) : undefined
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
      // 如果是比赛类型，将挑战赛ID添加到通知描述中
      if (formValue.type === 1 && formValue.challengeId) {
        notice.content = {
          ...notice.content,
          description: `${notice.content?.description || ''} [挑战赛ID:${formValue.challengeId}]`
        };
      }
      
      const createSystemNotificationRequest: CreateSystemNotificationRequest = {
        type: formValue.type,
        target: formValue.type === 2 ? formValue.targetIds : [],
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

  loadChallenges(): void {
    // 从后台获取挑战赛模板信息
    this.consoleService.getAllChallengeTemplates('').subscribe({
      next: (response) => {
        if (response.templates) {
          this.challenges = response.templates.map(template => ({
            id: template.id || 0,
            name: template.name || '',
            open_time: template.open_time || '',
            close_time: template.close_time || '',
            end_time: template.end_time || '',
            reward_remains: template.reward_remains || 0
          }));
        }
      },
      error: (error) => {
        console.error('获取挑战赛模板失败:', error);
        this.challenges = [];
      }
    });
  }

  getChallengeTemplate(challengeId: number): void {
    this.consoleService.getChallengeTemplate('', challengeId.toString()).subscribe({
      next: (response) => {
        if (response.template) {
          const template = response.template;
          console.log('Challenge template:', template);
          // 这里可以显示挑战赛的详细信息
          // 例如：开始时间、结束时间、奖励剩余时间等
        }
      },
      error: (error) => {
        console.error('Failed to get challenge template:', error);
      }
    });
  }

  onChallengeChange(event: any): void {
    const challengeId = event.target.value;
    if (challengeId) {
      this.getChallengeTemplate(parseInt(challengeId));
      // 从本地数据中获取挑战赛信息
      this.selectedChallenge = this.challenges.find(c => c.id == challengeId);
    } else {
      this.selectedChallenge = null;
    }
  }

  isNotificationEffective(notification: SystemNotice): boolean {
    if (!notification.effective_time) {
      return false;
    }
    const effectiveTime = new Date(notification.effective_time);
    const now = new Date();
    return effectiveTime <= now;
  }

  isNotificationExpired(notification: SystemNotice): boolean {
    if (!notification.expiry_time) {
      return false;
    }
    const expiryTime = new Date(notification.expiry_time);
    const now = new Date();
    return expiryTime <= now;
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


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

import {Component, inject, Injectable, OnInit, TemplateRef, ViewChild} from '@angular/core';
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
  prev_cursor?: string;
}

@Component({
  templateUrl: './system-notifications.component.html',
  styleUrls: ['./system-notifications.component.scss'],
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, NgbModule]
})
export class SystemNotificationsComponent implements OnInit {
  private today: NgbDate;
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
      challengeTimeType: [''], // 比赛时间类型选择
      targetIds: this.formBuilder.array([]),
      items: this.formBuilder.array([]),
      immediateSend: [true], // 默认选择立即发送
      enableExpiry: [false], // 设置过期时间
      effectiveDate: [{value: null, disabled: true}],
      effectiveTime: [{value: null, disabled: true}],
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

    this.notificationForm.get('immediateSend')!.valueChanges.subscribe(immediate => {
      if (immediate) {
        // 立即生效时只禁用生效时间控件，过期时间保持独立
        this.notificationForm.get('effectiveDate')!.disable();
        this.notificationForm.get('effectiveTime')!.disable();

        // 清除挑战赛时间类型选择
        this.notificationForm.get('challengeTimeType')!.setValue('');
      } else {
        // 不立即生效时启用生效时间控件
          this.notificationForm.get('effectiveDate')!.enable();
          this.notificationForm.get('effectiveTime')!.enable();
        }
    });

    // 监听发送类型变化
    this.notificationForm.get('type')!.valueChanges.subscribe(type => {
      if (type != 1) { // 如果不是比赛类型
        this.notificationForm.get('challengeId')!.setValue('');
        this.notificationForm.get('challengeTimeType')!.setValue('');
        this.selectedChallenge = null;
      } else {
        // 如果是比赛类型，默认不使用立即生效
        this.notificationForm.patchValue({
          immediateSend: false
        });
      }
    });

    // 触发初始状态设置
    this.notificationForm.get('immediateSend')!.updateValueAndValidity();
    this.notificationForm.get('enableExpiry')!.updateValueAndValidity();

    this.route.data.subscribe({
      next: (data) => {
        this.notifications.length = 0;
        if (data && data[0]) {
          const response = data[0];
          if (response.notice) {
            this.notifications.push(...response.notice);
          }
          this.totalCount = response.total_count || 0;
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
  public totalCount = 0;
  public notifications: SystemNotice[] = [];
  public nextCursor = '';
  public prevCursor = '';
  closeResult = '';
  showSuccess = false;
  showError = false;
  errorMessage = '';
  loading = false;

  statusFilter = '';
  searchQuery = '';
  currentCursor = '';
  isSearchMode = false;
  editingNotification: SystemNotice | null = null;

  defaultItems = [
    {id: '10000', name: '金币', icon: 'GoldCoin'},
    {id: '10001', name: '钻石', icon: 'Gem'},
    {id: '10002', name: '体力', icon: 'Strength'},
    {id: '20000', name: '广告券', icon: 'Coupon'},
  ];

  challenges: any[] = [];
  selectedChallenge: any = null;

  @ViewChild('errorModal') errorModal!: TemplateRef<any>;

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
      limit: 10,
      cursor: this.currentCursor,
    };

    this.fetchNotifications(params, true);
  }

  private loadNotificationsList(): void {
    const params: any = {
      limit: 10,
      cursor: this.currentCursor,
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
        this.nextCursor = response.next_cursor || '';
        this.prevCursor = response.prev_cursor || '';
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
      this.totalCount = d.total_count ?? 0;
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

  private toTimeString(date: NgbDateStruct | null, time: NgbTimeStruct | null): string | undefined {
    if (!date) {
      return undefined;
    }

    // 构建本地时间
    const localDate = new Date(
      date.year,
      date.month - 1,
      date.day,
      time?.hour || 0,
      time?.minute || 0,
      0
    );

    // 转换为UTC时间字符串
    return localDate.toISOString();
  }

  convertItemsToGameItems(): GameItem[] {
    const itemsArray = this.notificationForm.get('items') as FormArray;
    return itemsArray.controls.map((control: AbstractControl) => {
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

  getItemNameById(itemId: string): string {
    const item = this.defaultItems.find(item => item.id === itemId);
    return item ? item.name : itemId;
  }

  onImageError(event: Event): void {
    const target = event.target as HTMLImageElement;
    if (target) {
      target.style.display = 'none';
    }
  }

  operateAllowed(): boolean{
    // only admin and developers are allowed.
    const allowed = this.authService.sessionRole <= UserRole.USER_ROLE_DEVELOPER;
    // console.log('操作权限检查:', {
    //   sessionRole: this.authService.sessionRole,
    //   developerRole: UserRole.USER_ROLE_DEVELOPER,
    //   allowed: allowed
    // });
    return allowed;
  }

  openModal(content: any, notification?: SystemNotice): void {
    this.editingNotification = notification || null;
    if (notification) {
      // 检查是否为立即发送（生效时间等于创建时间）
      const isImmediateSend = notification.effective_time &&
        new Date(notification.effective_time).getTime() === new Date(notification.create_time!).getTime();

      // 获取通知类型，默认为0（全体）
      const notificationType = (notification as any).notice_type || 0;

      this.notificationForm.patchValue({
        type: notificationType,
        subject: notification.subject,
        desc: notification.content?.description || '',
        challengeId: notification.challenge_id || '',
        challengeTimeType: '', // 编辑时重置为空，让用户重新选择
        immediateSend: isImmediateSend,
        enableExpiry: !isImmediateSend && !!notification.expiry_time,
      });

      // 编辑时禁用类型选择
      this.notificationForm.get('type')!.disable();

      // 如果是比赛类型且有挑战赛ID，加载挑战赛信息
      if (notificationType === 1 && notification.challenge_id) {
        const challengeId = parseInt(notification.challenge_id.toString());
        this.getChallengeTemplate(challengeId);

        // 查找挑战赛信息，如果找不到则稍后再试
        this.selectedChallenge = this.challenges.find(c => c.id == challengeId) || null;

        // 如果没有找到挑战赛信息，可能是因为挑战赛列表还没有加载完成
        if (!this.selectedChallenge && this.challenges.length === 0) {
          // 延迟一点再尝试查找
          setTimeout(() => {
            this.selectedChallenge = this.challenges.find(c => c.id == challengeId) || null;
          }, 1000);
        }
      }

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
      const itemsArray = this.notificationForm.get('items') as FormArray;
      itemsArray.clear();
      if (notification.content?.rewards) {
        notification.content.rewards.forEach(reward => {
          itemsArray.push(this.formBuilder.group({
            id: [reward.id],
            num: [parseInt(String(reward.num || '1'), 10)],
          }));
        });
      }
    } else {
      this.notificationForm.reset({
        type: 0,
        subject: '',
        desc: '',
        challengeId: '',
        challengeTimeType: '',
        immediateSend: true,
        enableExpiry: false,
        effectiveDate: null,
        effectiveTime: null,
        expireDate: null,
        expireTime: null,
      });
      // 新建时启用类型选择
      this.notificationForm.get('type')!.enable();
      const itemsArray = this.notificationForm.get('items') as FormArray;
      itemsArray.clear();
      this.selectedChallenge = null;
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

    // 验证比赛类型必须选择挑战赛
    if (formValue.type == 1 && !formValue.challengeId) {
      this.showErrorModal('比赛类型通知必须选择挑战赛');
      return;
    }

    // 验证生效时间不能小于当前时间
    if (!formValue.immediateSend && formValue.effectiveDate) {
      // 构建完整的生效时间（包含日期和时间）
      const effectiveDateTime = new Date(
        formValue.effectiveDate.year,
        formValue.effectiveDate.month - 1,
        formValue.effectiveDate.day,
        formValue.effectiveTime?.hour || 0,
        formValue.effectiveTime?.minute || 0,
        0
      );
      const now = new Date();
      // 转换为UTC时间进行比较
      const effectiveUTC = effectiveDateTime.toISOString();
      const nowUTC = now.toISOString();
      if (effectiveUTC < nowUTC) {
        this.showErrorModal('生效时间不能小于当前时间');
        return;
      }
    }

    // 验证过期时间不能小于生效时间
    if (formValue.enableExpiry && formValue.expireDate && formValue.effectiveDate && !formValue.immediateSend) {
      // 构建完整的生效时间（包含日期和时间）
      const effectiveDateTime = new Date(
        formValue.effectiveDate.year,
        formValue.effectiveDate.month - 1,
        formValue.effectiveDate.day,
        formValue.effectiveTime?.hour || 0,
        formValue.effectiveTime?.minute || 0,
        0
      );
      // 构建完整的过期时间（包含日期和时间）
      const expireDateTime = new Date(
        formValue.expireDate.year,
        formValue.expireDate.month - 1,
        formValue.expireDate.day,
        formValue.expireTime?.hour || 0,
        formValue.expireTime?.minute || 0,
        0
      );
      // 转换为UTC时间进行比较
      const effectiveUTC = effectiveDateTime.toISOString();
      const expireUTC = expireDateTime.toISOString();
      if (expireUTC <= effectiveUTC) {
        this.showErrorModal('过期时间必须大于生效时间');
        return;
      }
    }

    // 处理立即发送逻辑
    let effectiveTime: string | undefined;
    if (formValue.immediateSend) {
      // 立即发送，使用当前时间作为生效时间
      const now = new Date();
      effectiveTime = now.toISOString();
    } else if (formValue.effectiveDate) {
      // 手动选择生效时间
      effectiveTime = this.toTimeString(formValue.effectiveDate, formValue.effectiveTime);
    } else {
      // 如果没有选择立即发送也没有设置生效时间，使用当前时间
      const now = new Date();
      effectiveTime = now.toISOString();
    }

    const notice: SystemNotice = {
      notice_type: formValue.type,
      subject: formValue.subject,
      content: {
        description: formValue.desc,
        rewards: this.convertItemsToGameItems(),
      },
      effective_time: effectiveTime,
      expiry_time: formValue.enableExpiry && formValue.expireDate ? this.toTimeString(formValue.expireDate, formValue.expireTime) : undefined
    };

    // 如果是比赛类型且有挑战赛ID，则添加挑战赛ID
    if (formValue.type == 1 && formValue.challengeId) {
      const challengeId = parseInt(formValue.challengeId);
      if (!isNaN(challengeId) && challengeId > 0) {
        notice.challenge_id = challengeId;
      }
    }

    // 构建更新请求数据
    const updateData: any = {
      content: notice.content,
      effective_time: notice.effective_time,
      expiry_time: notice.expiry_time,
      subject: notice.subject,
      challenge_id: notice.challenge_id
    };

    if (this.editingNotification?.id) {
        console.log('准备更新通知:', this.editingNotification.id, updateData);
        this.notificationsService.updateNotification(this.editingNotification.id, updateData).subscribe({
        next: (response) => {
          console.log('更新成功:', response);
          this.showSuccess = true;
          this.showError = false;
          this.errorMessage = '通知更新成功';
          setTimeout(() => this.showSuccess = false, 3000);
          this.search(0);
          this.modalService.dismissAll();
          this.loadNotifications();
        },
        error: (error) => {
          console.error('更新失败:', error);
          this.showSuccess = false;
          this.showErrorModal('更新失败: ' + (error.error?.message || error.message || '未知错误'));
          this.search(0);
        }
      });
    } else {
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
          this.showErrorModal('创建失败: ' + (error || '未知错误'));
          this.search(0);
          console.error('创建失败', error);
        }
      });
    }
  }

  deleteNotification(notification: SystemNotice): void {
    if (!notification.id) {
      console.error('通知ID为空，无法删除');
      return;
    }

    console.log('准备删除通知:', notification);

    this.deleteConfirmService.openDeleteConfirmModal(
      () => {
        console.log('用户确认删除，开始删除通知...');
        this.notificationsService.deleteNotification(notification.id!).subscribe({
          next: (response) => {
            console.log('删除成功:', response);
            this.showSuccess = true;
            this.errorMessage = '通知删除成功';
            setTimeout(() => this.showSuccess = false, 3000);
            this.loadNotifications();
          },
          error: (error) => {
            console.error('删除失败:', error);
            this.showErrorModal('删除失败: ' + (error.error?.message || error.message || '未知错误'));
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
    this.currentCursor = '';
    this.loadNotifications();
  }

  clearSearch(): void {
    this.searchForm.reset();
    this.searchQuery = '';
    this.statusFilter = '';
    this.isSearchMode = false;
    this.currentCursor = '';
    this.loadNotifications();
  }

  onStatusFilterChange(): void {
    this.isSearchMode = false;
    this.searchQuery = '';
    this.currentCursor = '';
    this.loadNotifications();
  }

  // 向前翻页
  onPreviousPage(): void {
    if (this.prevCursor) {
      this.currentCursor = this.prevCursor;
      this.loadNotifications();
    }
  }

  // 向后翻页
  onNextPage(): void {
    if (this.nextCursor) {
      this.currentCursor = this.nextCursor;
      this.loadNotifications();
    }
  }

  // 回到第一页
  onFirstPage(): void {
    this.currentCursor = '';
    this.loadNotifications();
  }

  // 检查是否有上一页
  hasPreviousPage(): boolean {
    return !!this.prevCursor;
  }

  // 检查是否有下一页
  hasNextPage(): boolean {
    return !!this.nextCursor;
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
    this.consoleService.getChallengeTemplate('', challengeId).subscribe({
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

  // 获取挑战赛时间选项
  getChallengeTimeOptions(): {value: string, label: string, time: string}[] {
    if (!this.selectedChallenge) {
      return [];
    }

    const options = [
      {
        value: 'open_time',
        label: '开始时间',
        time: this.selectedChallenge.open_time
      },
      {
        value: 'close_time',
        label: '结束时间',
        time: this.selectedChallenge.close_time
      },
      {
        value: 'end_time',
        label: '结算时间',
        time: this.selectedChallenge.end_time
      }
    ];

    // 计算关闭时间（结算时间 + 奖励剩余时间）
    if (this.selectedChallenge.end_time && this.selectedChallenge.reward_remains) {
      const endTime = new Date(this.selectedChallenge.end_time);
      const closeTime = new Date(endTime.getTime() + (this.selectedChallenge.reward_remains * 60 * 1000));

      options.push({
        value: 'final_close_time',
        label: '关闭时间',
        time: closeTime.toISOString()
      });
    }

    return options;
  }

  // 处理时间类型选择变化
  onChallengeTimeTypeChange(event: any): void {
    const timeType = event.target.value;
    if (timeType && this.selectedChallenge) {
      const timeOptions = this.getChallengeTimeOptions();
      const selectedOption = timeOptions.find(option => option.value === timeType);

      if (selectedOption) {
        // 自动取消立即生效
        this.notificationForm.patchValue({
          immediateSend: false
        });

        // 自动设置生效时间
        const selectedTime = new Date(selectedOption.time);

        // 设置日期和时间
        this.notificationForm.patchValue({
          effectiveDate: {
            year: selectedTime.getFullYear(),
            month: selectedTime.getMonth() + 1,
            day: selectedTime.getDate()
          },
          effectiveTime: {
            hour: selectedTime.getHours(),
            minute: selectedTime.getMinutes()
          }
        });
      }
    }
  }

  isNotificationEffective(notification: SystemNotice): boolean {
    if (!notification.effective_time) {
      console.log('通知无生效时间，允许操作:', notification.subject);
      return false;
    }
    const effectiveTime = new Date(notification.effective_time);
    const now = new Date();
    const isEffective = effectiveTime <= now;

    // console.log('通知生效状态检查:', {
    //   subject: notification.subject,
    //   effective_time: notification.effective_time,
    //   effectiveTime: effectiveTime,
    //   now: now,
    //   isEffective: isEffective
    // });

    // 如果生效时间已经过了，则认为通知已经生效，不能修改或删除
    return isEffective;
  }

  isNotificationExpired(notification: SystemNotice): boolean {
    if (!notification.expiry_time) {
      return false;
    }
    const expiryTime = new Date(notification.expiry_time);
    const now = new Date();
    return expiryTime <= now;
  }

  // 关闭错误弹窗
  closeErrorModal(): void {
    this.showError = false;
    this.errorMessage = '';
  }

  // 显示错误弹窗
  showErrorModal(errorMessage: string): void {
    this.errorMessage = errorMessage;
    this.showError = true;
    this.modalService.open(this.errorModal, {
      backdrop: 'static',
      keyboard: false,
      centered: true
    }).result.then(() => {
      this.showError = false;
      this.errorMessage = '';
    }, () => {
      this.showError = false;
      this.errorMessage = '';
    });
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



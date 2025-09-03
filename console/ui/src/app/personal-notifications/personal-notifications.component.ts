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
  PersonalNotificationLog,
  ListPersonalNotificationLogResponse, CreateSystemNotificationRequest, GameItem, NoticeContent, UserRole, GameReward, SystemNotice,
} from '../console.service';
import {DEFAULT_GAME_ITEMS} from '../shared/constants';
import {Observable} from 'rxjs';
import {FormBuilder, FormGroup, FormControl, FormArray, AbstractControl, Validators, ReactiveFormsModule} from '@angular/forms';
import {AuthenticationService} from '../authentication.service';
import {NgbModal, NgbCalendar, NgbDateStruct, NgbTimeStruct, NgbDate, NgbAlert, NgbModule} from '@ng-bootstrap/ng-bootstrap';
import {PersonalNotificationsService} from './personal-notifications.service';
import {DeleteConfirmService} from '../shared/delete-confirm.service';
import {CommonModule, NgOptimizedImage} from '@angular/common';

import {ModalDismissReasons} from '@ng-bootstrap/ng-bootstrap';

interface NotificationResponse {
  logs?: PersonalNotificationLog[];
  total_count?: number;
  next_cursor?: string;
  prev_cursor?: string;
}

@Component({
  templateUrl: './personal-notifications.component.html',
  styleUrls: ['./personal-notifications.component.scss'],
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, NgbModule, NgOptimizedImage]
})
export class PersonalNotificationsComponent implements OnInit {
  private today: NgbDate;
  notificationForm: FormGroup;
  items: FormArray;
  logs: PersonalNotificationLog[] = [];
  error: string = '';
  showSuccess = false;
  totalCount = 0;
  nextCursor = '';
  prevCursor = '';

  // 错误modal相关
  currentErrorMessage: string = '';
  @ViewChild('errorModalTemplate') errorModalTemplate!: TemplateRef<any>;

  // 默认道具列表
  readonly defaultItems = DEFAULT_GAME_ITEMS;

  // 缓存道具名称，避免重复计算
  private itemNameCache = new Map<string, string>();

  constructor(
    private readonly route: ActivatedRoute,
    private readonly router: Router,
    private readonly consoleService: ConsoleService,
    private readonly authService: AuthenticationService,
    private readonly formBuilder: FormBuilder,
    private readonly personalNotificationsService: PersonalNotificationsService,
    private readonly modalService: NgbModal,
    private readonly deleteConfirmService: DeleteConfirmService,
    private readonly calendar: NgbCalendar,
  ) {
    this.today = this.calendar.getToday();
    this.items = this.formBuilder.array([]);

    this.notificationForm = this.formBuilder.group({
      type: [2], // 固定为个人类型
      subject: [''],
      desc: [''],
      targetUsers: [''], // 改为单个字符串字段
      items: this.formBuilder.array([])
    });
  }

  ngOnInit(): void {
    this.refresh();
  }

  get n() {
    return this.notificationForm.controls;
  }

  // 计算属性：获取所有日志的显示数据，避免模板中重复调用方法
  get logsDisplayData() {
    return this.logs.map(log => {
      const rewardItems = this.getRewardItems(log);
      return {
        log,
        rewardItems: rewardItems.map(item => ({
          ...item,
          displayData: {
            icon: this.getIconById(item.id!),
            name: this.getItemNameById(item.id!)
          }
        }))
      };
    });
  }

  refresh(): void {
    // 清除缓存
    this.itemNameCache.clear();

    this.personalNotificationsService.getPersonalNotificationLogs({}).subscribe({
      next: (response: ListPersonalNotificationLogResponse) => {
        this.logs = response.logs || [];
        this.totalCount = response.total_count || 0;
        this.nextCursor = response.next_cursor || '';
        this.prevCursor = response.prev_cursor || '';
      },
      error: (error) => {
        console.error('加载日志失败:', error);
      }
    });
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

  openModal(content: TemplateRef<any>): void {
    // 重置表单
    this.notificationForm.reset({
      type: 2, // 固定为个人类型
      subject: '',
      desc: '',
      targetUsers: '', // 重置目标用户字符串
    });

    // 清空道具数组
    const items = this.notificationForm.get('items') as FormArray;
    items.clear();

    this.modalService.open(content, {ariaLabelledBy: 'modal-basic-title', size: 'xl'});
  }

  onSubmit(): void {
    if (!this.notificationForm.valid) {
      this.error = '请填写完整的表单信息';
      return;
    }

    const formValue = this.notificationForm.value;

    // 构建奖励列表 - 参考system-notifications的做法
    const items = formValue.items
      .filter((item: any) => item.id && item.num > 0)
      .map((item: any) => ({
        id: item.id,
        num: item.num
      }));

    // 构建奖励数据结构，与system-notifications保持一致
    const rewards = [{
      wallet: {
        ad: 0,
        coin: 0,
        gem: 0
      },
      items: items
    }];

    // 构建通知内容
    const content: NoticeContent = {
      description: formValue.desc,
      rewards: rewards
    };

    // 处理目标用户字符串，按空格分割并过滤空值
    const targetUsers = formValue.targetUsers
      ? formValue.targetUsers.trim().split(/\s+/).filter((username: string) => username.trim() !== '')
      : [];

    const request: CreateSystemNotificationRequest = {
      type: 2, // 个人类型
      notice: {
        subject: formValue.subject,
        content: content,
        notice_type: 0
      },
      target: targetUsers
    };

    this.personalNotificationsService.sendPersonalNotification(request).subscribe({
      next: (result) => {
        // 成功时关闭窗口并刷新数据
        this.modalService.dismissAll();
        this.refresh();

        // 显示成功消息
        this.showSuccess = true;
        this.error = '';

        // 3秒后隐藏成功消息
        setTimeout(() => {
          this.showSuccess = false;
        }, 3000);
      },
      error: (err) => {
        // 失败时显示错误modal
        this.showErrorModal(err || '发送失败');
      }
    });
  }

  // 显示错误信息的modal
  private showErrorModal(errorMessage: string): void {
    const errorModal = this.modalService.open(this.errorModalTemplate, {
      ariaLabelledBy: 'error-modal-title',
      size: 'sm',
      centered: true
    });

    // 将错误信息传递给modal
    this.currentErrorMessage = errorMessage;
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

  getRewardItems(log: PersonalNotificationLog): GameItem[] {
    if (!log.content?.rewards || log.content.rewards.length === 0) {
      return [];
    }
    return log.content.rewards[0].items || [];
  }

  getItemsControls(): AbstractControl[] {
    const items = this.notificationForm.get('items') as FormArray;
    return items ? items.controls : [];
  }

  getTargetIds(log: PersonalNotificationLog): string[] {
    if (!log.target_ids) {
      return [];
    }
    // 移除花括号，按逗号分割，过滤空值
    const cleanIds = log.target_ids.replace(/[{}]/g, '').trim();
    return cleanIds.split(',').filter(id => id.trim() !== '');
  }

  // 分页相关方法
  hasPreviousPage(): boolean {
    return !!this.prevCursor;
  }

  hasNextPage(): boolean {
    return !!this.nextCursor;
  }

  onFirstPage(): void {
    this.refresh();
  }

  onPreviousPage(): void {
    if (this.prevCursor) {
      // 清除缓存
      this.itemNameCache.clear();

      this.personalNotificationsService.getPersonalNotificationLogs({cursor: this.prevCursor}).subscribe({
        next: (response: ListPersonalNotificationLogResponse) => {
          this.logs = response.logs || [];
          this.totalCount = response.total_count || 0;
          this.nextCursor = response.next_cursor || '';
          this.prevCursor = response.prev_cursor || '';
        }
      });
    }
  }

  onNextPage(): void {
    if (this.nextCursor) {
      // 清除缓存
      this.itemNameCache.clear();

      this.personalNotificationsService.getPersonalNotificationLogs({cursor: this.nextCursor}).subscribe({
        next: (response: ListPersonalNotificationLogResponse) => {
          this.logs = response.logs || [];
          this.totalCount = response.total_count || 0;
          this.nextCursor = response.next_cursor || '';
          this.prevCursor = response.prev_cursor || '';
        }
      });
    }
  }
}


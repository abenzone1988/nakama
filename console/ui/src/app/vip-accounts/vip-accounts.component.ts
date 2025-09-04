// Copyright 2024 The Nakama Authors
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

import {Component, Injectable, OnInit, OnDestroy, ViewChild, TemplateRef} from '@angular/core';
import {ActivatedRoute, ActivatedRouteSnapshot, Resolve, Router, RouterStateSnapshot} from '@angular/router';
import {VipAccountList, VipAccount, ConsoleService, UserRole, AddVipAccountRequest, AddVipAccountResponse, VipAccountError} from '../console.service';
import {Observable, Subject} from 'rxjs';
import {UntypedFormBuilder, UntypedFormGroup, Validators, ReactiveFormsModule} from '@angular/forms';
import {AuthenticationService} from '../authentication.service';
import {DeleteConfirmService} from '../shared/delete-confirm.service';
import {takeUntil} from 'rxjs/operators';
import {NgbModal, NgbModule} from '@ng-bootstrap/ng-bootstrap';
import {CommonModule} from '@angular/common';

@Component({
  templateUrl: './vip-accounts.component.html',
  styleUrls: ['./vip-accounts.component.scss'],
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, NgbModule]
})
export class VipAccountsComponent implements OnInit, OnDestroy {
  public error = '';
  public success = '';
  public vipAccountsCount = 0;
  public vipAccounts: Array<VipAccount> = [];
  public nextCursor = '';
  public prevCursor = '';
  public searchForm: UntypedFormGroup;
  public addVipForm: UntypedFormGroup;
  public querySubject: Subject<void>;
  public ongoingQuery = false;
  public showAddVipModal = false;

  // 错误modal相关
  currentErrorMessage: string = '';
  @ViewChild('errorModalTemplate') errorModalTemplate!: TemplateRef<any>;

  constructor(
    private readonly route: ActivatedRoute,
    private readonly router: Router,
    private readonly consoleService: ConsoleService,
    private readonly authService: AuthenticationService,
    private readonly formBuilder: UntypedFormBuilder,
    private readonly deleteConfirmService: DeleteConfirmService,
    private readonly modalService: NgbModal,
  ) {}

  ngOnInit(): void {
    this.querySubject = new Subject<void>();
    this.searchForm = this.formBuilder.group({
      filter: [''],
    });

    this.addVipForm = this.formBuilder.group({
      usernames: ['', Validators.required], // 改为支持多个用户名输入
      expire_time: [''],
    });

    const qp = this.route.snapshot.queryParamMap;
    this.f.filter.setValue(qp.get('filter') || '');
    this.nextCursor = qp.get('cursor') || '';

    if (this.nextCursor && this.nextCursor !== '') {
      this.search(1);
    } else if (this.f.filter.value) {
      this.search(0);
    }

    this.route.data.subscribe(
      d => {
        this.vipAccounts.length = 0;
        if (d && d[0]) {
          const accounts = d[0].accounts || [];
          this.vipAccounts.push(...accounts);
          this.vipAccountsCount = d[0].total_count || 0;
          this.nextCursor = d[0].next_cursor || '';
          this.prevCursor = d[0].prev_cursor || '';
        }
      },
      err => {
        this.error = err;
      });
  }

  ngOnDestroy(): void {
    this.querySubject.next();
    this.querySubject.complete();
  }

  search(state: number): void {
    if (this.ongoingQuery) {
      this.querySubject.next();
    }
    this.ongoingQuery = true;

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

    this.consoleService.listVipAccounts('', this.f.filter.value, cursor, 50)
      .pipe(takeUntil(this.querySubject))
      .subscribe(d => {
        this.error = '';

        this.vipAccounts.length = 0;
        this.vipAccounts.push(...(d.accounts || []));
        this.vipAccountsCount = d.total_count || 0;
        this.nextCursor = d.next_cursor || '';
        this.prevCursor = d.prev_cursor || '';

        this.router.navigate([], {
          relativeTo: this.route,
          queryParams: {
            filter: this.f.filter.value,
            cursor
          },
          queryParamsHandling: 'merge',
        });
        this.ongoingQuery = false;
      }, err => {
        this.error = err;
        this.ongoingQuery = false;
      });
  }

  cancelQuery(): void {
    this.querySubject.next();
    this.ongoingQuery = false;
  }

  openAddVipModal(content: any): void {
    this.showAddVipModal = true;
    this.addVipForm.reset();
    this.error = '';
    this.success = '';
    this.modalService.open(content, {ariaLabelledBy: 'modal-basic-title'});
  }

  addVipAccount(): void {
    if (this.addVipForm.invalid) {
      return;
    }

    // 处理多个用户名，按空格分割并过滤空值
    const usernames = this.addVipFormControls.usernames.value
      .trim()
      .split(/\s+/)
      .filter((username: string) => username.trim() !== '');

    if (usernames.length === 0) {
      this.error = '请输入有效的用户名';
      return;
    }

    // 构建请求
    const request: AddVipAccountRequest = {
      usernames: usernames,
    };

    // 如果设置了过期时间，转换为ISO字符串
    if (this.addVipFormControls.expire_time.value) {
      const expireDate = new Date(this.addVipFormControls.expire_time.value);
      request.expire_time = expireDate.toISOString();
    }

    // 发送批量添加请求
    this.consoleService.addVipAccount('', request).subscribe(
      (response: AddVipAccountResponse) => {
        // 处理成功的账户
        if (response.success_accounts) {
          response.success_accounts.forEach(account => {
            this.vipAccounts.unshift(account);
            this.vipAccountsCount++;
          });
        }

        // 显示结果
        this.showBatchResultFromResponse(response);

        // 关闭模态框并重置表单
        this.modalService.dismissAll();
        this.addVipForm.reset();
      },
      (err) => {
        this.error = err;
        this.success = '';
        console.error('批量添加VIP失败:', err);
      }
    );
  }

  // 从响应中显示批量操作结果
  private showBatchResultFromResponse(response: AddVipAccountResponse): void {
    const successCount = response.success_count || 0;
    const failedCount = response.failed_count || 0;
    const totalCount = response.total_count || 0;

    if (failedCount === 0) {
      this.success = `成功添加 ${successCount} 个VIP用户`;
      this.error = '';
    } else if (successCount === 0) {
      this.error = `添加失败，所有 ${totalCount} 个用户都无法添加`;
      this.success = '';

      // 显示详细的失败信息
      if (response.failed_accounts && response.failed_accounts.length > 0) {
        const failedDetails = response.failed_accounts.map(failed =>
          `${failed.username}: ${failed.error_message}`
        ).join('\n');
        this.error = `添加失败的详细信息：\n${failedDetails}`;
      }
    } else {
      this.success = `成功添加 ${successCount} 个VIP用户`;
      this.error = `有 ${failedCount} 个用户添加失败`;

      // 显示详细的失败信息
      if (response.failed_accounts && response.failed_accounts.length > 0) {
        const failedDetails = response.failed_accounts.map(failed =>
          `${failed.username}: ${failed.error_message}`
        ).join('\n');
        console.warn('添加VIP失败的详细信息:', failedDetails);
      }
    }
  }

  removeVipAccount(event: any, i: number, account: VipAccount): void {
    this.deleteConfirmService.openDeleteConfirmModal(
      () => {
        event.target.disabled = true;
        event.preventDefault();
        this.error = '';
        this.consoleService.removeVipAccount('', account.user_id || '').subscribe(() => {
          this.success = '成功移除VIP用户';
          this.error = '';
          // 更新本地数据，将VIP设为失效
          this.vipAccounts[i].is_active = false;
          this.vipAccounts[i].expiry_time = new Date().toISOString();
        }, err => {
          this.error = err;
          this.success = '';
          event.target.disabled = false;
        }, );
      }
    ,null, "确认删除", "确认删除vip权限？");
  }

  viewAccount(account: VipAccount): void {
    this.router.navigate(['/accounts', account.user_id]);
  }

  isVipActive(account: VipAccount): boolean {
    if (!account.expiry_time) return false;
    const expireDate = new Date(account.expiry_time);
    return expireDate > new Date();
  }

  formatDate(dateString: string | undefined): string {
    if (!dateString) return '';
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN');
  }

   get f(): any {
    return this.searchForm.controls;
  }

  get addVipFormControls(): any {
    return this.addVipForm.controls;
  }
}

@Injectable({providedIn: 'root'})
export class VipAccountsResolver implements Resolve<VipAccountList> {
  constructor(private readonly consoleService: ConsoleService) {}

  resolve(route: ActivatedRouteSnapshot, state: RouterStateSnapshot): Observable<VipAccountList> {
    const filter = route.queryParamMap.get('filter');
    const cursor = route.queryParamMap.get('cursor');

    return this.consoleService.listVipAccounts('', filter || undefined, cursor || undefined, 50);
  }
}

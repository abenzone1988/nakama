import { Component, OnInit } from '@angular/core';
import { ConsoleService, UserRole, Announcement } from '../console.service';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { NgbModal } from '@ng-bootstrap/ng-bootstrap';
import { AnnouncementsService } from './announcements.service';
import { DeleteConfirmService } from '../shared/delete-confirm.service';

@Component({
  templateUrl: './announcements.component.html'
})
export class AnnouncementsComponent implements OnInit {
  announcements: Announcement[] = [];
  loading = false;
  totalCount = 0;
  currentPage = 1;
  pageSize = 10;
  statusFilter = '';
  cursor = '';

  form: FormGroup;
  editingAnnouncement: Announcement | null = null;

  constructor(
    private readonly consoleService: ConsoleService,
    private readonly announcementsService: AnnouncementsService,
    private readonly modalService: NgbModal,
    private readonly fb: FormBuilder,
    private readonly deleteConfirmService: DeleteConfirmService,
  ) {
    this.form = this.fb.group({
      title: ['', Validators.required],
      content: ['', Validators.required],
      img: [''],
      status: [0, Validators.required],
    });
  }

  ngOnInit(): void {
    this.loadAnnouncements();
  }

  loadAnnouncements(): void {
    this.loading = true;
    const params: any = {
      limit: this.pageSize,
      cursor: this.cursor,
    };
    if (this.statusFilter !== '') {
      params['status'] = parseInt(this.statusFilter, 10);
    }

    this.announcementsService.getAnnouncements(params).subscribe({
      next: (response) => {
        this.announcements = response.announcements || [];
        this.totalCount = response.total_count || 0;
        this.cursor = response.next_cursor || '';
        this.loading = false;
      },
      error: (error) => {
        console.error('加载公告列表失败', error);
        this.loading = false;
      }
    });
  }

  openModal(content: any, announcement?: Announcement): void {
    this.editingAnnouncement = announcement || null;
    if (announcement) {
      this.form.patchValue(announcement);
    } else {
      this.form.reset({status: 0});
    }
    const modalRef = this.modalService.open(content, {
      size: 'lg',
      windowClass: 'announcement-modal',
      backdropClass: 'announcement-backdrop',
      modalDialogClass: 'announcement-dialog'
    });

    // 手动设置 z-index
    setTimeout(() => {
      const backdrop = document.querySelector('.announcement-backdrop') as HTMLElement;
      const modal = document.querySelector('.announcement-modal') as HTMLElement;
      if (backdrop) {
        backdrop.style.zIndex = '1040';
      }
      if (modal) {
        modal.style.zIndex = '1050';
      }
    });
  }

  onSubmit(): void {
    if (this.form.invalid) {
      return;
    }

    const data = this.form.value;
    
    if (this.editingAnnouncement) {
      this.announcementsService.updateAnnouncement(this.editingAnnouncement.id || '', data).subscribe({
        next: () => {
          alert('更新成功');
          this.modalService.dismissAll();
          this.loadAnnouncements();
        },
        error: (error) => {
          console.error('更新失败', error);
          alert('更新失败');
        }
      });
    } else {
      this.announcementsService.createAnnouncement(data).subscribe({
        next: () => {
          alert('创建成功');
          this.modalService.dismissAll();
          this.loadAnnouncements();
        },
        error: (error) => {
          console.error('创建失败', error);
          alert('创建失败');
        }
      });
    }
  }

  deleteAnnouncement(announcement: Announcement): void {
    this.deleteConfirmService.openDeleteConfirmModal(
      () => {
        this.announcementsService.deleteAnnouncement(announcement.id || '').subscribe({
          next: () => {
            alert('删除成功');
            this.loadAnnouncements();
          },
          error: (error) => {
            console.error('删除失败', error);
            alert('删除失败');
          }
        });
      },
      undefined,
      '删除公告',
      `确认删除标题为 "${announcement.title}" 的公告吗？`
    );
  }

  onPageChange(page: number): void {
    this.currentPage = page;
    this.loadAnnouncements();
  }

  onStatusFilterChange(): void {
    this.currentPage = 1;
    this.cursor = '';
    this.loadAnnouncements();
  }

  getStatusText(status: number): string {
    const statusMap = {
      0: '草稿',
      1: '已发布',
      2: '已下线'
    };
    return statusMap[status] || '未知';
  }

  getStatusClass(status: number): string {
    const classMap = {
      0: 'badge-secondary',
      1: 'badge-success',
      2: 'badge-danger'
    };
    return classMap[status] || 'badge-secondary';
  }
} 
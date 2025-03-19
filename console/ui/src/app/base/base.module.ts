import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule } from '@angular/router';
import { NgbNavModule } from '@ng-bootstrap/ng-bootstrap';
import { BaseComponent } from './base.component';
import { FilterByGroupPipe } from './filter-by-group.pipe';

@NgModule({
  declarations: [
    BaseComponent,
    FilterByGroupPipe
  ],
  imports: [
    CommonModule,
    RouterModule,
    NgbNavModule
  ],
  exports: [
    BaseComponent
  ]
})
export class BaseModule { } 
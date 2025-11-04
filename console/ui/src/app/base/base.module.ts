import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule } from '@angular/router';
import { NgbNavModule } from '@ng-bootstrap/ng-bootstrap';
import { BaseComponent } from './base.component';
import { FilterByGroupPipe } from './filter-by-group.pipe';
import { FormsModule } from '@angular/forms';
import { HttpClientModule } from '@angular/common/http';

@NgModule({
  declarations: [
    BaseComponent,
    FilterByGroupPipe
  ],
  imports: [
    CommonModule,
    RouterModule,
    NgbNavModule,
    FormsModule,
    HttpClientModule
  ],
  exports: [
    BaseComponent
  ]
})
export class BaseModule { } 
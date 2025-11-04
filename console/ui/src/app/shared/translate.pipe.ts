import { Pipe, PipeTransform, OnDestroy } from '@angular/core';
import { TranslationService } from '../services/translation.service';
import { Subscription } from 'rxjs';

@Pipe({
  name: 'translate',
  pure: false
})
export class TranslatePipe implements PipeTransform, OnDestroy {
  private subscription: Subscription;
  private currentValue: string = '';
  private currentKey: string = '';

  constructor(private translationService: TranslationService) {
    this.subscription = this.translationService.getCurrentLang().subscribe(() => {
      if (this.currentKey) {
        this.currentValue = this.translationService.translate(this.currentKey);
      }
    });
  }

  transform(value: string): string {
    this.currentKey = value;
    this.currentValue = this.translationService.translate(value);
    return this.currentValue;
  }

  ngOnDestroy(): void {
    if (this.subscription) {
      this.subscription.unsubscribe();
    }
  }
}

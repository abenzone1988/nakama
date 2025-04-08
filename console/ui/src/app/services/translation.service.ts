import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { BehaviorSubject, Observable, of } from 'rxjs';
import { catchError } from 'rxjs/operators';

@Injectable({
  providedIn: 'root'
})
export class TranslationService {
  private currentLang = new BehaviorSubject<string>('en');
  private translations = new BehaviorSubject<{[key: string]: string}>({});
  private translationsCache = new Map<string, {[key: string]: string}>();

  constructor(private http: HttpClient) {
    // 从 localStorage 恢复语言设置
    const savedLang = localStorage.getItem('language') || 'en';
    this.setLanguage(savedLang);
  }

  setLanguage(lang: string) {
    localStorage.setItem('language', lang);
    this.currentLang.next(lang);

    // 如果是英文，直接使用空对象作为翻译（使用原始文本）
    if (lang === 'en') {
      this.translations.next({});
      return;
    }

    // 检查缓存中是否已有该语言的翻译
    if (this.translationsCache.has(lang)) {
      this.translations.next(this.translationsCache.get(lang) || {});
      return;
    }

    // 加载其他语言的翻译文件
    this.loadTranslations(lang);
  }

  private loadTranslations(lang: string) {
    this.http.get<{[key: string]: string}>(`/assets/i18n/${lang}.json`)
      .pipe(
        catchError(error => {
          console.error(`Failed to load translations for ${lang}`, error);
          return of({});
        })
      )
      .subscribe(translations => {
        // 缓存翻译结果
        this.translationsCache.set(lang, translations);
        this.translations.next(translations);
      });
  }

  translate(key: string): string {
    const currentLang = this.currentLang.getValue();
    const translations = this.translations.getValue();

    // 如果是英文或没有对应的翻译，返回原始文本
    if (currentLang === 'en' || !translations[key]) {
      return key;
    }

    return translations[key] || key;
  }

  getCurrentLang(): Observable<string> {
    return this.currentLang.asObservable();
  }
}

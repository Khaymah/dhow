import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { Ar } from '@/I18n/Locales/Ar';
import { En } from '@/I18n/Locales/En';
import { I18nProvider } from '@/I18n/I18nProvider';
import { STORAGE_KEY } from '@/I18n/Locale';
import { translate } from '@/I18n/Translate';
import { useI18n } from '@/I18n/useI18n';

const placeholders = (text: string): string[] =>
  [...new Set([...text.matchAll(/\{(\w+)(?:\}|,\s*plural,)/g)].map((match) => match[1] ?? ''))].sort();
const PLURAL_WORDS = new Set(['zero', 'one', 'two', 'few', 'many', 'other']);

describe('locale catalogs', () => {
  it('have exactly the same keys in Arabic and English', () => {
    expect(Object.keys(En).sort()).toEqual(Object.keys(Ar).sort());
  });

  it('use the same placeholders in both languages and never ship an empty message', () => {
    for (const [key, arabic] of Object.entries(Ar)) {
      const english = En[key as keyof typeof En];
      expect(placeholders(english), key).toEqual(placeholders(arabic));
      expect(arabic.trim(), key).not.toBe('');
      expect(english.trim(), key).not.toBe('');
    }
  });

  it('write the Arabic catalog in Arabic script', () => {
    const arabicLetters = /[؀-ۿ]/;
    const untranslated = Object.entries(Ar)
      .filter(([key, text]) => key !== 'language.en' && !arabicLetters.test(text))
      .map(([key]) => key);
    expect(untranslated).toEqual([]);
  });
});

describe('the UI glossary (ADR-0034)', () => {
  const ALLOWED_LATIN = new Set(['English', 'AGPL', 'commit', 'DHOW_DATABASE_URL', 'DHOW_', 'API', 'Git', 'GitHub', 'GitLab', 'SSH', 'HTTPS']);

  it('keeps English words out of the Arabic copy, except the language name, one first-mention term and a technical value', () => {
    const stray = Object.entries(Ar).flatMap(([key, text]) =>
      [...text.matchAll(/\{\w+\}|\{\w+,\s*plural,|[A-Za-z][A-Za-z0-9_]+/g)]
        .map((match) => match[0])
        .filter((word) => !word.startsWith('{') && !PLURAL_WORDS.has(word) && !ALLOWED_LATIN.has(word))
        .map((word) => `${key}: ${word}`),
    );
    expect(stray).toEqual([]);
  });

  it('puts the English term in parentheses on first mention only', () => {
    const withParentheses = Object.entries(Ar)
      .filter(([, text]) => /\([A-Za-z]+\)/.test(text))
      .map(([key]) => key);
    expect(withParentheses).toEqual(['status.build.commit']);
  });

  it('names the shell sections and groups with the terms of ADR-0059', () => {
    expect(Ar['shell.nav.home']).toBe('الرئيسية');
    expect(Ar['shell.nav.jobs']).toBe('التشغيلات');
    expect(Ar['shell.nav.runners']).toBe('أجهزة التشغيل');
    expect(Ar['shell.nav.members']).toBe('الأعضاء');
    expect(Ar['shell.nav.orgSettings']).toBe('إعدادات المؤسسة');
    expect(Ar['shell.nav.audit']).toBe('سجل الأحداث');
    expect(Ar['shell.nav.system']).toBe('حالة السيرفر');
    expect([Ar['shell.group.run'], Ar['shell.group.org'], Ar['shell.group.watch']]).toEqual(['النشر', 'المؤسسة', 'المتابعة']);
    expect(Ar['shell.bottom.runners']).toBe('الأجهزة');
    expect(Ar['appearance.title']).toBe('المظهر والألوان');
    expect(En['shell.nav.home']).toBe('Home');
    expect(En['shell.nav.runners']).toBe('Runners');
    expect(En['shell.nav.audit']).toBe('Audit log');
  });

  it('no longer calls the home page a workspace', () => {
    for (const text of Object.values(Ar)) expect(text).not.toContain('مساحة الشغل');
    for (const text of Object.values(En)) expect(text.toLowerCase()).not.toContain('workspace');
  });

  it('names projects in the shell from M4 on', () => {
    expect(Ar['shell.nav.projects']).toBe('المشاريع');
    expect(En['shell.nav.projects']).toBe('Projects');
    expect(Object.keys(Ar).filter((key) => key.startsWith('shell.') && /soon/i.test(key))).toEqual([]);
  });
});

describe('translate', () => {
  it('fills placeholders and leaves unknown ones visible', () => {
    expect(translate('en', 'shell.language.switch', { language: 'Arabic' })).toBe('Switch the language to \u2068Arabic\u2069');
    expect(translate('en', 'shell.language.switch')).toBe('Switch the language to {language}');
  });

  it('isolates every value, so a path or an address keeps its order inside Arabic text', () => {
    const path = '.github/ci.yaml';
    const url = 'ssh://git@git.example.com/team/app.git';
    const text = translate('ar', 'projects.form.path.hint', { ext: '.yaml', example: path });
    expect(text).toContain(`\u2068${path}\u2069`);
    expect(text).toContain('\u2068.yaml\u2069');
    expect(translate('ar', 'repository.source.url.hint', { ssh: url, scp: 'git@h:a/b.git' })).toContain(`\u2068${url}\u2069`);
  });
});

const Probe = () => {
  const { Locale, SetLocale, t } = useI18n();
  return (
    <>
      <p>{t('home.title')}</p>
      <output>{Locale}</output>
      <button
        type="button"
        onClick={() => {
          SetLocale(Locale === 'ar' ? 'en' : 'ar');
        }}
      >
        flip
      </button>
    </>
  );
};

describe('I18nProvider', () => {
  it('starts in Arabic, right to left', () => {
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    expect(screen.getByText(Ar['home.title'])).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('ar');
    expect(document.documentElement.dir).toBe('rtl');
    expect(document.title).toBe(Ar['app.title']);
  });

  it('flips lang, dir and the title together and remembers the choice', async () => {
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    await userEvent.click(screen.getByRole('button', { name: 'flip' }));
    expect(screen.getByText(En['home.title'])).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('en');
    expect(document.documentElement.dir).toBe('ltr');
    expect(document.title).toBe(En['app.title']);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('en');
    await userEvent.click(screen.getByRole('button', { name: 'flip' }));
    expect(document.documentElement.dir).toBe('rtl');
  });

  it('restores the saved language on the next visit', () => {
    window.localStorage.setItem(STORAGE_KEY, 'en');
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    expect(screen.getByRole('status')).toHaveTextContent('en');
    expect(document.documentElement.dir).toBe('ltr');
  });

  it('falls back to Arabic for an unsupported saved language', () => {
    window.localStorage.setItem(STORAGE_KEY, 'fr');
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    expect(screen.getByRole('status')).toHaveTextContent('ar');
  });

  it('keeps working when storage is blocked', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    expect(screen.getByRole('status')).toHaveTextContent('ar');
    await userEvent.click(screen.getByRole('button', { name: 'flip' }));
    expect(screen.getByRole('status')).toHaveTextContent('en');
    expect(document.documentElement.dir).toBe('ltr');
  });
});

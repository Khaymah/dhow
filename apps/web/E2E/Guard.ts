import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';

import { Ar } from '../Src/I18n/Locales/Ar';
import { En } from '../Src/I18n/Locales/En';
import type { TMessages } from '../Src/I18n/Messages';
import { freshCode, setLanguage, type ISession, type IUser } from './Stack';

export type TLang = 'ar' | 'en';

export interface IView {
  Context: BrowserContext;
  Page: Page;
  T: TMessages;
  Lang: TLang;
  Problems: string[];
  Violations: string[];
}

export const MESSAGES: Record<TLang, TMessages> = { ar: Ar, en: En };
export const DESKTOP = { width: 1440, height: 900 };
export const PHONE = { width: 390, height: 844 };
const VIOLATION_BINDING = 'dhowViolation';

export const open = async (browser: Browser, lang: TLang, viewport: { width: number; height: number }): Promise<IView> => {
  const phone = viewport.width < 600;
  const context = await browser.newContext({ viewport, isMobile: phone, hasTouch: phone });
  const violations: string[] = [];
  await context.exposeFunction(VIOLATION_BINDING, (detail: string) => {
    violations.push(detail);
  });
  await context.addInitScript(
    ({ value, binding }) => {
      window.localStorage.setItem('dhow.lang', value);
      document.addEventListener('securitypolicyviolation', (event) => {
        const send = Reflect.get(window, binding) as (detail: string) => void;
        send(`${event.disposition} ${event.violatedDirective} ${event.blockedURI} ${event.sample}`);
      });
    },
    { value: lang, binding: VIOLATION_BINDING },
  );
  const page = await context.newPage();
  const problems: string[] = [];
  page.on('console', (message) => {
    if (message.type() === 'error') problems.push(`console: ${message.text()}`);
  });
  page.on('pageerror', (error) => problems.push(`page error: ${error.message}`));
  return { Context: context, Page: page, T: MESSAGES[lang], Lang: lang, Problems: problems, Violations: violations };
};

export const expectClean = async (view: IView): Promise<void> => {
  await expect(view.Page.locator('style')).toHaveCount(0);
  expect(view.Violations).toEqual([]);
  expect(view.Problems).toEqual([]);
  expect(await view.Page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
};

export const expectNowhere = async (view: IView, value: string): Promise<void> => {
  const seen = await view.Page.evaluate(() => {
    const storage = (store: Storage) => Array.from({ length: store.length }, (_, i) => `${store.key(i) ?? ''}=${store.getItem(store.key(i) ?? '') ?? ''}`);
    const fields = Array.from(document.querySelectorAll('input, textarea')).map((element) => (element as HTMLInputElement).value);
    return [window.location.href, JSON.stringify(window.history.state), ...storage(window.localStorage), ...storage(window.sessionStorage), ...fields, document.documentElement.outerHTML].join('\n');
  });
  expect(seen.includes(value)).toBe(false);
};

export const signInPage = async (view: IView, user: IUser, session: ISession): Promise<void> => {
  const { Page: page, T } = view;
  await setLanguage(session, view.Lang);
  await page.goto('/login');
  await page.getByRole('textbox', { name: T['field.email'], exact: true }).fill(user.Email);
  await page.getByRole('textbox', { name: T['field.password'], exact: true }).fill(user.Password);
  await page.getByRole('button', { name: T['login.submit'], exact: true }).click();
  if (user.TotpSecret !== undefined) {
    await page.getByRole('textbox', { name: T['mfa.step.code'] }).fill(await freshCode(user.TotpSecret));
    await page.getByRole('button', { name: T['mfa.step.submit'] }).click();
  }
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
  await expect(page.locator('html')).toHaveAttribute('lang', view.Lang);
};

export const shot = async (view: IView, path: string): Promise<void> => {
  await view.Page.screenshot({ path, fullPage: true });
};

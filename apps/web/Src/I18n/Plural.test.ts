import { describe, expect, it } from 'vitest';

import { pluralCategory, selectPlurals } from '@/I18n/Plural';
import { translate, isolate } from '@/I18n/Translate';

const JOBS_AR =
  '{count, plural, zero {لا توجد تشغيلات} one {تشغيل واحد} two {تشغيلان} few {{count} تشغيلات} many {{count} تشغيلًا} other {{count} تشغيل}}';
const JOBS_EN = '{count, plural, one {{count} job} other {{count} jobs}}';

const fill = (text: string, count: number) => text.replaceAll('{count}', String(count));

describe('plural forms', () => {
  it.each([
    [0, 'zero', 'لا توجد تشغيلات'],
    [1, 'one', 'تشغيل واحد'],
    [2, 'two', 'تشغيلان'],
    [3, 'few', '3 تشغيلات'],
    [11, 'many', '11 تشغيلًا'],
    [100, 'other', '100 تشغيل'],
  ] as const)('picks the Arabic form for %i (%s)', (count, category, text) => {
    expect(pluralCategory('ar', count)).toBe(category);
    expect(fill(selectPlurals('ar', JOBS_AR, { count }), count)).toBe(text);
  });

  it.each([
    [1, 'one', '1 job'],
    [2, 'other', '2 jobs'],
    [0, 'other', '0 jobs'],
  ] as const)('picks the English form for %i (%s)', (count, category, text) => {
    expect(pluralCategory('en', count)).toBe(category);
    expect(fill(selectPlurals('en', JOBS_EN, { count }), count)).toBe(text);
  });

  it('prefers an exact match, then the category, then other', () => {
    const text = '{count, plural, =0 {never} one {once} other {{count} times}}';
    expect(selectPlurals('en', text, { count: 0 })).toBe('never');
    expect(selectPlurals('en', text, { count: 1 })).toBe('once');
    expect(selectPlurals('ar', text, { count: 2 })).toBe('{count} times');
  });

  it('keeps the text around a plural and reads any parameter name', () => {
    const text = 'Try again in {seconds, plural, one {1 second} other {{seconds} seconds}}. Then sign in.';
    expect(selectPlurals('en', text, { seconds: 1 })).toBe('Try again in 1 second. Then sign in.');
    expect(selectPlurals('en', text, { seconds: 30 })).toBe('Try again in {seconds} seconds. Then sign in.');
  });

  it('falls back to other without a count and leaves a plain text alone', () => {
    expect(selectPlurals('ar', JOBS_AR)).toBe('{count} تشغيل');
    expect(selectPlurals('ar', 'العدد: {count}', { count: 3 })).toBe('العدد: {count}');
  });

  it('fills the chosen form through translate', () => {
    expect(translate('ar', 'runners.facts.capacity.value', { count: 1 })).toBe('تشغيل واحد في نفس الوقت');
    expect(translate('ar', 'runners.facts.capacity.value', { count: 3 })).toBe(`${isolate('3')} تشغيلات في نفس الوقت`);
    expect(translate('en', 'runners.facts.capacity.value', { count: 1 })).toBe(`${isolate('1')} job at a time`);
    expect(translate('en', 'runners.facts.capacity.value', { count: 3 })).toBe(`${isolate('3')} jobs at a time`);
  });
});

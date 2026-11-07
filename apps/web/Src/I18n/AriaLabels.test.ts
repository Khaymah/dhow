import { describe, expect, it } from 'vitest';

import { Ar } from '@/I18n/Locales/Ar';
import { En } from '@/I18n/Locales/En';

const mismatches = (catalog: Record<string, string>): string[] =>
  Object.keys(catalog)
    .filter((key) => key.endsWith('.aria'))
    .filter((key) => {
      const visible = catalog[key.slice(0, -'.aria'.length)];
      const label = catalog[key];
      return visible !== undefined && label !== undefined && !label.includes(visible);
    });

describe('accessible names keep the visible text', () => {
  it.each([
    ['ar', Ar],
    ['en', En],
  ])('every %s .aria label contains the text of the button it names', (_lang, catalog) => {
    expect(mismatches(catalog as Record<string, string>)).toEqual([]);
  });
});

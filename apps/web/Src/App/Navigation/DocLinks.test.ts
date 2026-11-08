import { existsSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { SECTIONS } from '@/App/Navigation/Sections';
import { DOC_LINKS, docLinkFor } from '@/Core/Help/DocLinks';
import { docsUrl } from '@/Core/Help/DocsUrl';
import { Ar } from '@/I18n/Locales/Ar';
import { En } from '@/I18n/Locales/En';

const DOCS = join(process.cwd(), '..', '..', 'docs');

describe('DocLinks', () => {
  it.each(SECTIONS.map((section) => [section.id, section.path] as const))('%s (%s) has a help link', (id) => {
    expect(docLinkFor(id), `add ${id} to Core/Help/DocLinks.ts`).not.toBeNull();
  });

  it('has no link for a page that does not exist', () => {
    const ids = new Set<string>(SECTIONS.map((section) => section.id));
    expect(Object.keys(DOC_LINKS).filter((id) => !ids.has(id))).toEqual([]);
  });

  it.each(Object.entries(DOC_LINKS))('%s opens %s, which exists in docs/ and docs/en/', (_id, path) => {
    expect(existsSync(join(DOCS, path)), `${path} in the Arabic docs`).toBe(true);
    expect(existsSync(join(DOCS, 'en', path)), `${path} in the English docs`).toBe(true);
    expect(docsUrl(path, 'ar')).toMatch(new RegExp(`/docs/${path.replace('.', '\\.')}$`));
    expect(docsUrl(path, 'en')).toMatch(new RegExp(`/docs/en/${path.replace('.', '\\.')}$`));
  });

  it('has a hint line for every page in both languages', () => {
    for (const section of SECTIONS) {
      const key = `help.hint.${section.id}` as keyof typeof Ar;
      expect(Ar[key], `Ar ${key}`).toBeTruthy();
      expect(En[key], `En ${key}`).toBeTruthy();
    }
  });
});

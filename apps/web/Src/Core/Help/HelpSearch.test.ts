import { describe, expect, it } from 'vitest';

import { HelpContentAr } from '@/Core/Help/Generated/HelpContentAr';
import { HelpContentEn } from '@/Core/Help/Generated/HelpContentEn';
import { buildIndex, normalizeText, plainText, queryTerms, searchEntries } from '@/Core/Help/HelpSearch';
import { loadHelpContent } from '@/Core/Help/LoadHelpContent';

const english = buildIndex(HelpContentEn);
const arabic = buildIndex(HelpContentAr);
const titles = (hits: ReturnType<typeof searchEntries>) => hits.map((hit) => hit.Entry.Title);

describe('normalizeText', () => {
  it('ignores Arabic diacritics, hamza shapes, ta marbuta and tatweel', () => {
    expect(normalizeText('الإِعْدَاد')).toBe(normalizeText('الاعداد'));
    expect(normalizeText('مؤسسة')).toBe(normalizeText('مؤسسه'));
    expect(normalizeText('أداة')).toBe(normalizeText('اداه'));
    expect(normalizeText('كـــلمة')).toBe(normalizeText('كلمة'));
  });

  it('ignores case and Latin accents', () => {
    expect(normalizeText('Café')).toBe('cafe');
    expect(normalizeText('DHOW_Public_URL')).toBe('dhow_public_url');
  });
});

describe('queryTerms', () => {
  it('splits on spaces and punctuation', () => {
    expect(queryTerms('  setup-link, please?? ')).toEqual(['setup', 'link', 'please']);
    expect(queryTerms('   ')).toEqual([]);
  });
});

describe('searchEntries over the embedded content', () => {
  it('keeps everything when the query is empty', () => {
    expect(searchEntries(english, '')).toHaveLength(english.length);
    expect(english.length).toBeGreaterThan(10);
  });

  it('finds an English answer by a word in its text', () => {
    expect(titles(searchEntries(english, 'cookie'))).toContain('Why does sign-in succeed and then ask me to sign in again?');
  });

  it('finds an Arabic answer, with or without diacritics', () => {
    const plain = titles(searchEntries(arabic, 'الاعداد'));
    expect(plain.length).toBeGreaterThan(0);
    expect(titles(searchEntries(arabic, 'الإِعْدَاد'))).toEqual(plain);
  });

  it('needs every word to match', () => {
    expect(searchEntries(english, 'cookie zzzzqqqq')).toEqual([]);
    expect(searchEntries(english, 'zzzzqqqq')).toEqual([]);
  });

  it('puts a title match before a body match', () => {
    const hits = searchEntries(english, 'approval');
    expect(hits[0]?.Entry.Title.toLowerCase()).toContain('approval');
  });

  it('searches the troubleshooting sections too', () => {
    const hits = searchEntries(english, 'DHOW_TRUSTED_PROXIES');
    expect(hits.some((hit) => hit.Kind === 'troubleshooting')).toBe(true);
  });

  it('finds a command inside a code block', () => {
    expect(searchEntries(english, 'grep setup_url').length).toBeGreaterThan(0);
  });
});

describe('the embedded content', () => {
  it.each([
    ['ar', HelpContentAr],
    ['en', HelpContentEn],
  ] as const)('has the same questions in %s', (_locale, content) => {
    expect(content.Faq.map((group) => group.Entries.length)).toEqual(HelpContentEn.Faq.map((group) => group.Entries.length));
    expect(content.Troubleshooting).toHaveLength(HelpContentEn.Troubleshooting.length);
    expect(content.Releases[0]?.Sections[0]?.Kind).toBe('highlights');
  });

  it('has an answer for every question', () => {
    for (const item of [...english, ...arabic]) expect(plainText(item.Entry.Body).length).toBeGreaterThan(10);
  });

  it('loads per language', async () => {
    expect(await loadHelpContent('en')).toBe(HelpContentEn);
    expect(await loadHelpContent('ar')).toBe(HelpContentAr);
  });
});

import { describe, expect, it } from 'vitest';

import { DOCS_BASE_URL, docsUrl } from '@/Core/Help/DocsUrl';

describe('docsUrl', () => {
  it('picks the folder from the UI language', () => {
    expect(docsUrl('guides/secrets.md#versions', 'en')).toBe(
      'https://github.com/Khaymah/dhow/blob/main/docs/en/guides/secrets.md#versions',
    );
    expect(docsUrl('guides/secrets.md', 'ar')).toBe('https://github.com/Khaymah/dhow/blob/main/docs/guides/secrets.md');
  });

  it('encodes an Arabic anchor', () => {
    expect(docsUrl('guides/secrets.md#الإصدارات', 'ar')).toBe(
      `${DOCS_BASE_URL}docs/guides/secrets.md#${encodeURIComponent('الإصدارات')}`,
    );
  });

  it.each([
    'https://evil.example/a.md',
    '//evil.example/a.md',
    '/etc/passwd.md',
    '../secrets.md',
    'guides/../../x.md',
    'javascript:alert(1).md',
    'guides/secrets.html',
    'guides/secrets.md#a b',
    '',
  ])('rejects %s', (path) => {
    expect(docsUrl(path, 'en')).toBeNull();
  });
});

import type { TLocale } from '@/I18n/Locale';

export const DOCS_BASE_URL = 'https://github.com/Khaymah/dhow/blob/main/';

const DOCS_PATH = /^[a-z0-9][a-z0-9/_-]*\.md(?:#[^\s#]+)?$/;
const DOCS_FOLDER: Record<TLocale, string> = { ar: 'docs/', en: 'docs/en/' };

export const docsUrl = (path: string, locale: TLocale): string | null => {
  if (!DOCS_PATH.test(path) || path.includes('..') || path.includes('//')) return null;
  return encodeURI(`${DOCS_BASE_URL}${DOCS_FOLDER[locale]}${path}`);
};

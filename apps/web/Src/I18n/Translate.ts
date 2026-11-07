import type { TLocale } from '@/I18n/Locale';
import { BootAr } from '@/I18n/Locales/Boot/Ar';
import type { TMessageKey, TMessageParams, TMessages, TWorkspaceMessages } from '@/I18n/Messages';
import { selectPlurals } from '@/I18n/Plural';
import { useCatalogStore } from '@/I18n/useCatalogStore';

type TCatalog = Readonly<Partial<TMessages>>;

const bootOf = (locale: TLocale): TCatalog => useCatalogStore.getState().Boot[locale] ?? BootAr;

const loadedWorkspace = (locale: TLocale): TWorkspaceMessages | undefined =>
  useCatalogStore.getState().Workspace[locale];

export const FIRST_STRONG_ISOLATE = '\u2068';
export const POP_DIRECTIONAL_ISOLATE = '\u2069';

export const isolate = (value: string | number): string => `${FIRST_STRONG_ISOLATE}${String(value)}${POP_DIRECTIONAL_ISOLATE}`;

export const translate = (
  locale: TLocale,
  key: TMessageKey,
  params?: TMessageParams,
  workspace: TCatalog | undefined = loadedWorkspace(locale),
  boot: TCatalog = bootOf(locale),
): string => {
  const template = selectPlurals(locale, boot[key] ?? workspace?.[key] ?? key, params);
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (placeholder, name: string) => {
    const value = params[name];
    return value === undefined ? placeholder : isolate(value);
  });
};

export const isMessageKey = (key: string): key is TMessageKey =>
  Object.hasOwn(BootAr, key) ||
  Object.values(useCatalogStore.getState().Workspace).some((messages) => Object.hasOwn(messages, key));

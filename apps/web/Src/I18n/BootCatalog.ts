import type { TLocale } from '@/I18n/Locale';
import type { TBootMessages } from '@/I18n/Messages';
import { useCatalogStore } from '@/I18n/useCatalogStore';

const LOADERS: Partial<Record<TLocale, () => Promise<TBootMessages>>> = {
  en: () => import('@/I18n/Locales/Boot/En').then((module) => module.BootEn),
};

export const loadBootMessages = async (locale: TLocale): Promise<void> => {
  const load = LOADERS[locale];
  if (load === undefined || useCatalogStore.getState().Boot[locale] !== undefined) return;
  const messages = await load();
  useCatalogStore.setState((state) => ({ Boot: { ...state.Boot, [locale]: messages } }));
};

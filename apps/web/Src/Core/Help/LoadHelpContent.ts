import type { IHelpContent } from '@/Core/Help/HelpTypes';
import type { TLocale } from '@/I18n/Locale';

export const loadHelpContent = (locale: TLocale): Promise<IHelpContent> =>
  locale === 'ar'
    ? import('@/Core/Help/Generated/HelpContentAr').then((module) => module.HelpContentAr)
    : import('@/Core/Help/Generated/HelpContentEn').then((module) => module.HelpContentEn);

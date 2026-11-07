import { isApiError } from '@/Core/Api/ApiError';
import { docsUrl } from '@/Core/Help/DocsUrl';
import { useI18n } from '@/I18n/useI18n';

export interface IApiErrorHelp {
  Fix: string;
  HelpHref: string | null;
}

export const NO_ERROR_HELP: IApiErrorHelp = { Fix: '', HelpHref: null };

export const useApiErrorHelp = (): ((error: unknown) => IApiErrorHelp) => {
  const { Locale } = useI18n();
  return (error) => {
    if (!isApiError(error)) return NO_ERROR_HELP;
    const sameLanguage = error.Language === '' || error.Language === Locale;
    const path = sameLanguage ? error.DocsPath : error.DocsPath.split('#')[0] ?? '';
    return { Fix: error.Fix, HelpHref: path === '' ? null : docsUrl(path, Locale) };
  };
};

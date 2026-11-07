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
    return { Fix: error.Fix, HelpHref: error.DocsPath === '' ? null : docsUrl(error.DocsPath, Locale) };
  };
};

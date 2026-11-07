import { API_ERROR_INVALID_RESPONSE, API_ERROR_NETWORK, isApiError } from '@/Core/Api/ApiError';
import { useT } from '@/I18n/useT';

export const useApiErrorMessage = (): ((error: unknown) => string) => {
  const t = useT();
  return (error) => {
    if (!isApiError(error)) return t('api.error.unknown');
    if (error.Code === API_ERROR_NETWORK) return t('api.error.network');
    if (error.Code === API_ERROR_INVALID_RESPONSE) return t('api.error.invalid');
    return error.Message === '' ? t('api.error.unknown') : error.Message;
  };
};

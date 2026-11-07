import { isApiError } from '@/Core/Api/ApiError';
import { useApiErrorMessage } from '@/Core/Hooks/useApiErrorMessage';

export const useApiErrorText = (): ((error: unknown) => string) => {
  const message = useApiErrorMessage();
  return (error) => {
    const text = message(error);
    return isApiError(error) && error.Fix !== '' ? `${text} ${error.Fix}` : text;
  };
};

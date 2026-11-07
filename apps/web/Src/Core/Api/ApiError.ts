export type TApiFieldError = Readonly<{ Field: string; Message: string }>;

export type TApiError = Readonly<{
  Code: string;
  Message: string;
  Fix?: string;
  DocsPath?: string;
  Details?: readonly TApiFieldError[];
}>;

export const API_ERROR_NETWORK = 'NETWORK';
export const API_ERROR_INVALID_RESPONSE = 'INVALID_RESPONSE';
export const API_ERROR_UNAUTHENTICATED = 'UNAUTHENTICATED';
export const API_ERROR_CSRF_FAILED = 'CSRF_FAILED';

export class ApiError extends Error implements TApiError {
  readonly Code: string;
  readonly Message: string;
  readonly Fix: string;
  readonly DocsPath: string;
  readonly Status: number;
  readonly Details: readonly TApiFieldError[];
  readonly RetryAfterSeconds: number | null;

  constructor(error: TApiError, status: number, retryAfterSeconds: number | null = null) {
    super(error.Message);
    this.name = 'ApiError';
    this.Code = error.Code;
    this.Message = error.Message;
    this.Fix = typeof error.Fix === 'string' ? error.Fix : '';
    this.DocsPath = typeof error.DocsPath === 'string' ? error.DocsPath : '';
    this.Status = status;
    this.Details = error.Details ?? [];
    this.RetryAfterSeconds = retryAfterSeconds;
  }
}

export const isApiError = (value: unknown): value is ApiError => value instanceof ApiError;

export const isUnauthenticated = (value: unknown): boolean =>
  isApiError(value) && value.Status === 401 && value.Code === API_ERROR_UNAUTHENTICATED;

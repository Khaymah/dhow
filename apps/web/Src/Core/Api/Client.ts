import createClient, { type Middleware } from 'openapi-fetch';

import {
  API_ERROR_CSRF_FAILED,
  API_ERROR_INVALID_RESPONSE,
  API_ERROR_NETWORK,
  ApiError,
  type TApiError,
} from '@/Core/Api/ApiError';
import type { components, paths } from '@/Core/Api/Schema';
import { useCsrfStore } from '@/Core/Store/useCsrfStore';

export const API_BASE_PATH = '/api/v1';
export const CSRF_HEADER = 'X-CSRF-Token';
const DEFAULT_LANGUAGE = 'ar';
const SAFE_METHODS: readonly string[] = ['GET', 'HEAD', 'OPTIONS'];

const acceptLanguage: Middleware = {
  onRequest({ request }) {
    request.headers.set('Accept-Language', document.documentElement.lang || DEFAULT_LANGUAGE);
    return request;
  },
};

const csrfToken: Middleware = {
  onRequest({ request }) {
    const token = useCsrfStore.getState().Token;
    if (token !== null && !SAFE_METHODS.includes(request.method.toUpperCase())) {
      request.headers.set(CSRF_HEADER, token);
    }
    return request;
  },
};

export const createApiClient = (fetchImpl: typeof fetch = (...args) => globalThis.fetch(...args)) => {
  const client = createClient<paths>({
    baseUrl: `${window.location.origin}${API_BASE_PATH}`,
    fetch: fetchImpl,
    credentials: 'same-origin',
  });
  client.use(acceptLanguage);
  client.use(csrfToken);
  return client;
};

export const ApiClient = createApiClient();

export type TPageMeta = components['schemas']['PageMeta'];

interface IEnvelopeBody<TData> {
  Success: boolean;
  Data?: TData;
  Meta?: TPageMeta;
  Error?: TApiError;
}

export interface IFetchResult<TData> {
  data?: IEnvelopeBody<TData>;
  error?: unknown;
  response: Response;
}

export interface IUnwrapOptions {
  acceptFailureData?: boolean;
}

export interface IPage<TItem> {
  Items: TItem[];
  Meta: TPageMeta;
}

const isEnvelope = (value: unknown): value is IEnvelopeBody<unknown> =>
  typeof value === 'object' && value !== null && 'Success' in value;

const retryAfterOf = (response: Response): number | null => {
  const raw = response.headers.get('Retry-After');
  if (raw === null) return null;
  const seconds = Number(raw);
  return Number.isFinite(seconds) && seconds > 0 ? Math.ceil(seconds) : null;
};

const failure = (body: unknown, response: Response): ApiError => {
  if (isEnvelope(body) && body.Error) return new ApiError(body.Error, response.status, retryAfterOf(response), response.headers.get('Content-Language') ?? '');
  return new ApiError({ Code: API_ERROR_INVALID_RESPONSE, Message: '' }, response.status);
};

const succeeded = (body: unknown, options: IUnwrapOptions): body is IEnvelopeBody<unknown> =>
  isEnvelope(body) && body.Data !== undefined && (body.Success || options.acceptFailureData === true);

export const unwrap = <TData>(result: IFetchResult<TData>, options: IUnwrapOptions = {}): TData => {
  const body: unknown = result.data ?? result.error;
  if (succeeded(body, options)) return body.Data as TData;
  throw failure(body, result.response);
};

const toApiError = (error: unknown): ApiError => {
  if (error instanceof ApiError) return error;
  const code = error instanceof SyntaxError ? API_ERROR_INVALID_RESPONSE : API_ERROR_NETWORK;
  return new ApiError({ Code: code, Message: '' }, 0);
};

const refreshCsrfToken = async (): Promise<boolean> => {
  try {
    const me = unwrap(await ApiClient.GET('/me'));
    useCsrfStore.getState().Set(me.CsrfToken);
    return true;
  } catch {
    return false;
  }
};

const withCsrfRetry = async <TResult>(attempt: () => Promise<TResult>): Promise<TResult> => {
  try {
    return await attempt();
  } catch (error) {
    const csrfFailed = error instanceof ApiError && error.Code === API_ERROR_CSRF_FAILED;
    if (!csrfFailed || useCsrfStore.getState().Token === null) throw error;
    if (!(await refreshCsrfToken())) throw error;
    return attempt();
  }
};

export const request = <TData>(call: () => Promise<IFetchResult<TData>>, options?: IUnwrapOptions): Promise<TData> =>
  withCsrfRetry(async () => {
    try {
      return unwrap(await call(), options);
    } catch (error) {
      throw toApiError(error);
    }
  });

export const requestPage = <TItem>(call: () => Promise<IFetchResult<TItem[]>>): Promise<IPage<TItem>> =>
  withCsrfRetry(async () => {
    try {
      const result = await call();
      const items = unwrap(result);
      const body: unknown = result.data;
      const meta = isEnvelope(body) ? body.Meta : undefined;
      return { Items: items, Meta: meta ?? { Page: 1, Limit: items.length, Total: items.length } };
    } catch (error) {
      throw toApiError(error);
    }
  });

export const requestVoid = (call: () => Promise<IFetchResult<unknown>>): Promise<void> =>
  withCsrfRetry(async () => {
    try {
      const result = await call();
      const body: unknown = result.data ?? result.error;
      if (isEnvelope(body) && body.Success) return;
      throw failure(body, result.response);
    } catch (error) {
      throw toApiError(error);
    }
  });

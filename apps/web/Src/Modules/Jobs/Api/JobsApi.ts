import { API_BASE_PATH, ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';

type TSchemas = components['schemas'];

export type TJob = TSchemas['Job'];
export type TJobDetail = TSchemas['JobDetail'];
export type TJobSpec = TSchemas['JobSpec'];
export type TJobAttempt = TSchemas['JobAttempt'];
export type TJobRun = TSchemas['JobRun'];
export type TJobAttemptStep = TSchemas['JobAttemptStep'];
export type TJobState = TSchemas['JobState'];
export type TJobAttemptState = TSchemas['JobAttemptState'];
export type TJobStepState = TSchemas['JobStepState'];
export type TJobLogChunk = TSchemas['JobLogChunk'];
export type TJobLogStatus = TSchemas['JobLogStatus'];
export type TJobLogStream = TSchemas['JobLogStream'];

export const listJobs = (
  organizationId: string,
  page: number,
  limit: number,
  states: readonly TJobState[],
): Promise<IPage<TJob>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/jobs', {
      params: { path: { id: organizationId }, query: { Page: page, Limit: limit, ...(states.length === 0 ? {} : { State: [...states] }) } },
    }),
  );

export const getJob = (organizationId: string, jobId: string): Promise<TJobDetail> =>
  request(() => ApiClient.GET('/organizations/{id}/jobs/{jobId}', { params: { path: { id: organizationId, jobId } } }));

export const cancelJob = (organizationId: string, jobId: string): Promise<TJob> =>
  request(() => ApiClient.POST('/organizations/{id}/jobs/{jobId}/cancel', { params: { path: { id: organizationId, jobId } } }));

export type TJobLogPage = TSchemas['JobLogPage'];

export const LOG_PAGE_LIMIT = 500;

export const getJobLogPage = (organizationId: string, jobId: string, attempt: number, fromSeq: number): Promise<TJobLogPage> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/jobs/{jobId}/logs', {
      params: { path: { id: organizationId, jobId }, query: { Attempt: attempt, FromSeq: fromSeq, Limit: LOG_PAGE_LIMIT } },
    }),
  );

export const jobLogRawUrl = (organizationId: string, jobId: string, attempt: number): string =>
  `${API_BASE_PATH}/organizations/${encodeURIComponent(organizationId)}/jobs/${encodeURIComponent(jobId)}/logs/raw?Attempt=${String(attempt)}`;

export const jobLogStreamUrl =(organizationId: string, jobId: string, attempt: number, fromSeq: number): string => {
  const path = `${API_BASE_PATH}/organizations/${encodeURIComponent(organizationId)}/jobs/${encodeURIComponent(jobId)}/logs/stream`;
  const query = new URLSearchParams({ Attempt: String(attempt) });
  if (fromSeq > 1) query.set('FromSeq', String(fromSeq));
  return `${window.location.origin}${path}?${query.toString()}`;
};

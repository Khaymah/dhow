import { API_BASE_PATH, ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';

type TSchemas = components['schemas'];

export type TRun = TSchemas['Run'];
export type TRunState = TSchemas['RunState'];
export type TRunTrigger = TSchemas['RunTrigger'];
export type TRunDetail = TSchemas['RunDetail'];
export type TRunJob = TSchemas['RunJob'];
export type TRunJobState = TSchemas['RunJobState'];
export type TRunIssue = TSchemas['RunIssue'];
export type TArtifact = TSchemas['Artifact'];
export type TManualInputs = TSchemas['ManualInputs'];
export type TManualInput = TSchemas['ManualInput'];
export type TTriggerRun = TSchemas['TriggerRunRequest'];

export const RUNS_PAGE_SIZE = 20;

export const runsKey = (organizationId: string, projectId: string) => ['runs', organizationId, projectId] as const;
export const runListKey = (organizationId: string, projectId: string) => [...runsKey(organizationId, projectId), 'list'] as const;
export const runKey = (organizationId: string, projectId: string, runId: string) =>
  [...runsKey(organizationId, projectId), 'detail', runId] as const;
export const artifactsKey = (organizationId: string, projectId: string, runId: string) =>
  [...runsKey(organizationId, projectId), 'artifacts', runId] as const;
export const manualInputsKey = (organizationId: string, projectId: string) =>
  [...runsKey(organizationId, projectId), 'manual-inputs'] as const;

const runPath = (id: string, projectId: string, runId: string) => ({ params: { path: { id, projectId, runId } } });

export const listRuns = (
  organizationId: string,
  projectId: string,
  page: number,
  states: readonly TRunState[],
): Promise<IPage<TRun>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/runs', {
      params: {
        path: { id: organizationId, projectId },
        query: { Page: page, Limit: RUNS_PAGE_SIZE, ...(states.length === 0 ? {} : { State: [...states] }) },
      },
    }),
  );

export const getRun = (organizationId: string, projectId: string, runId: string): Promise<TRunDetail> =>
  request(() => ApiClient.GET('/organizations/{id}/projects/{projectId}/runs/{runId}', runPath(organizationId, projectId, runId)));

export const cancelRun = (organizationId: string, projectId: string, runId: string): Promise<TRunDetail> =>
  request(() => ApiClient.POST('/organizations/{id}/projects/{projectId}/runs/{runId}/cancel', runPath(organizationId, projectId, runId)));

export const rerunFailed = (organizationId: string, projectId: string, runId: string): Promise<TRunDetail> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/runs/{runId}/rerun-failed', runPath(organizationId, projectId, runId)),
  );

export const approveRun = (organizationId: string, projectId: string, runId: string): Promise<TRunDetail> =>
  request(() => ApiClient.POST('/organizations/{id}/projects/{projectId}/runs/{runId}/approve', runPath(organizationId, projectId, runId)));

export const triggerRun = (organizationId: string, projectId: string, body: TTriggerRun): Promise<TRun> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/runs', { params: { path: { id: organizationId, projectId } }, body }),
  );

export const getManualInputs = (organizationId: string, projectId: string): Promise<TManualInputs> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/manual-inputs', { params: { path: { id: organizationId, projectId } } }),
  );

export const listArtifacts = (organizationId: string, projectId: string, runId: string): Promise<TArtifact[]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/runs/{runId}/artifacts', runPath(organizationId, projectId, runId)),
  );

export const artifactDownloadHref = (organizationId: string, projectId: string, artifactId: string): string =>
  `${API_BASE_PATH}/organizations/${encodeURIComponent(organizationId)}/projects/${encodeURIComponent(projectId)}/artifacts/${encodeURIComponent(artifactId)}/download`;

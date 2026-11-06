import { ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';
import { environmentsKey } from '@/Modules/Environments/Api/EnvironmentsApi';

type TSchemas = components['schemas'];

export type TTarget = TSchemas['DeployTarget'];
export type THostCheck = TSchemas['DeployTargetHostCheck'];
export type TTargetCheck = TSchemas['DeployTargetCheck'];
export type TCheckStarted = TSchemas['DeployTargetCheckStarted'];
export type TTargetReleases = TSchemas['DeployTargetReleases'];
export type TTargetRelease = TSchemas['DeployTargetRelease'];
export type TReleaseHost = TSchemas['DeployTargetReleaseHost'];

export const TARGETS_LIMIT = 100;

export const targetsKey = (organizationId: string, projectId: string, environmentName: string) =>
  [...environmentsKey(organizationId, projectId), 'targets', environmentName] as const;
export const targetCheckKey = (organizationId: string, projectId: string, environmentName: string, name: string) =>
  [...targetsKey(organizationId, projectId, environmentName), 'check', name] as const;
export const targetReleasesKey = (organizationId: string, projectId: string, environmentName: string, name: string) =>
  [...targetsKey(organizationId, projectId, environmentName), 'releases', name] as const;

export const targetKey = (organizationId: string, projectId: string, environmentName: string, name: string) =>
  [...targetsKey(organizationId, projectId, environmentName), 'one', name] as const;

const targetPath = (id: string, projectId: string, environmentName: string, targetName: string) => ({ id, projectId, environmentName, targetName });

export const listTargets = (organizationId: string, projectId: string, environmentName: string): Promise<IPage<TTarget>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets', {
      params: { path: { id: organizationId, projectId, environmentName }, query: { Page: 1, Limit: TARGETS_LIMIT } },
    }),
  );

export const getTarget = (organizationId: string, projectId: string, environmentName: string, name: string): Promise<TTarget> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}', {
      params: { path: targetPath(organizationId, projectId, environmentName, name) },
    }),
  );

export const getTargetCheck = (organizationId: string, projectId: string, environmentName: string, name: string): Promise<TTargetCheck> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/check', {
      params: { path: targetPath(organizationId, projectId, environmentName, name) },
    }),
  );

export const startTargetCheck = (organizationId: string, projectId: string, environmentName: string, name: string): Promise<TCheckStarted> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/check', {
      params: { path: targetPath(organizationId, projectId, environmentName, name) },
    }),
  );

export const listTargetReleases = (organizationId: string, projectId: string, environmentName: string, name: string): Promise<TTargetReleases> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/releases', {
      params: { path: targetPath(organizationId, projectId, environmentName, name) },
    }),
  );

export const rollbackTarget = (
  organizationId: string,
  projectId: string,
  environmentName: string,
  name: string,
  releaseId: string,
  factor: TCurrentFactor,
): Promise<TCheckStarted> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/rollback', {
      params: { path: targetPath(organizationId, projectId, environmentName, name) },
      body: { ReleaseId: releaseId, CurrentFactor: factor },
    }),
  );

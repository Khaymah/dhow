import { ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';

type TSchemas = components['schemas'];

export type TEnvironment = TSchemas['Environment'];
export type TEnvironmentRules = TSchemas['EnvironmentRules'];
export type TEnvironmentRulesInput = TSchemas['EnvironmentRulesInput'];
export type TEnvironmentRef = TSchemas['EnvironmentRef'];
export type TRefKind = TSchemas['EnvironmentRefKind'];
export type TDeployment = TSchemas['Deployment'];
export type TDeploymentState = TSchemas['DeploymentState'];
export type TReviewerMember = TSchemas['Member'];
export type TReviewerTeam = TSchemas['Team'];

export const DEPLOYMENTS_PAGE_SIZE = 20;
export const DEPLOYED_SCAN_SIZE = 100;
const CHOICES_LIMIT = 100;

export const environmentsKey = (organizationId: string, projectId: string) => ['environments', organizationId, projectId] as const;
export const environmentListKey = (organizationId: string, projectId: string) =>
  [...environmentsKey(organizationId, projectId), 'list'] as const;
export const environmentKey = (organizationId: string, projectId: string, name: string) =>
  [...environmentsKey(organizationId, projectId), 'detail', name] as const;
export const deploymentsKey = (organizationId: string, projectId: string, name: string) =>
  [...environmentsKey(organizationId, projectId), 'deployments', name] as const;
export const deployedKey = (organizationId: string, projectId: string, name: string) =>
  [...environmentsKey(organizationId, projectId), 'deployed', name] as const;
export const reviewerChoicesKey = (organizationId: string) => ['environments', organizationId, 'reviewer-choices'] as const;

export interface ICreateEnvironment {
  Name: string;
  Description: string;
  Rules: TEnvironmentRulesInput;
}

export interface IUpdateEnvironment {
  Version: number;
  Description?: string;
  Rules?: TEnvironmentRulesInput;
}

const projectPath = (id: string, projectId: string) => ({ id, projectId });
const environmentPath = (id: string, projectId: string, environmentName: string) => ({ id, projectId, environmentName });

export const listEnvironments = (organizationId: string, projectId: string): Promise<TEnvironment[]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments', {
      params: { path: projectPath(organizationId, projectId), query: { Page: 1, Limit: 100 } },
    }),
  );

export const getEnvironment = (organizationId: string, projectId: string, name: string): Promise<TEnvironment> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}', {
      params: { path: environmentPath(organizationId, projectId, name) },
    }),
  );

export const createEnvironment = (
  organizationId: string,
  projectId: string,
  input: ICreateEnvironment,
  factor: TCurrentFactor,
): Promise<TEnvironment> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments', {
      params: { path: projectPath(organizationId, projectId) },
      body: { Name: input.Name, Description: input.Description, Rules: input.Rules, CurrentFactor: factor },
    }),
  );

export const updateEnvironment = (
  organizationId: string,
  projectId: string,
  name: string,
  input: IUpdateEnvironment,
  factor: TCurrentFactor,
): Promise<TEnvironment> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}', {
      params: { path: environmentPath(organizationId, projectId, name) },
      body: { ...input, CurrentFactor: factor },
    }),
  );

export const removeEnvironment = (organizationId: string, projectId: string, name: string, factor: TCurrentFactor): Promise<TEnvironment> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/remove', {
      params: { path: environmentPath(organizationId, projectId, name) },
      body: { CurrentFactor: factor },
    }),
  );

export const pauseEnvironment = (organizationId: string, projectId: string, name: string, reason: string): Promise<TEnvironment> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/pause', {
      params: { path: environmentPath(organizationId, projectId, name) },
      body: { Reason: reason },
    }),
  );

export const resumeEnvironment = (organizationId: string, projectId: string, name: string, factor: TCurrentFactor): Promise<TEnvironment> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/resume', {
      params: { path: environmentPath(organizationId, projectId, name) },
      body: { CurrentFactor: factor },
    }),
  );

export const listDeployments = (
  organizationId: string,
  projectId: string,
  name: string,
  page: number,
  limit: number,
): Promise<IPage<TDeployment>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}/deployments', {
      params: { path: environmentPath(organizationId, projectId, name), query: { Page: page, Limit: limit } },
    }),
  );

export const listReviewerMembers = (organizationId: string): Promise<TReviewerMember[]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/members', {
      params: { path: { id: organizationId }, query: { Page: 1, Limit: CHOICES_LIMIT } },
    }),
  );

export const listReviewerTeams = (organizationId: string): Promise<TReviewerTeam[]> =>
  request(() => ApiClient.GET('/organizations/{id}/teams', { params: { path: { id: organizationId } } }));

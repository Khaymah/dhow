import { ApiClient, request } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';

type TSchemas = components['schemas'];

export interface IEnvironmentScope {
  OrganizationId: string;
  ProjectId: string;
  EnvironmentName: string;
}

const LIST_QUERY = { Page: 1, Limit: 500 };

const base = (scope: IEnvironmentScope) => ({
  id: scope.OrganizationId,
  projectId: scope.ProjectId,
  environmentName: scope.EnvironmentName,
});

export const listEnvironmentSecrets = (scope: IEnvironmentScope): Promise<TSchemas['Secret'][]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}/secrets', {
      params: { path: base(scope), query: LIST_QUERY },
    }),
  );

export const putEnvironmentSecret = (
  scope: IEnvironmentScope,
  name: string,
  body: { Value: string; Description: string; CurrentFactor?: TCurrentFactor },
): Promise<TSchemas['Secret']> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}/secrets/{secretName}', {
      params: { path: { ...base(scope), secretName: name } },
      body,
    }),
  );

export const removeEnvironmentSecret = (scope: IEnvironmentScope, name: string): Promise<TSchemas['Secret']> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/secrets/{secretName}/remove', {
      params: { path: { ...base(scope), secretName: name } },
    }),
  );

export const listEnvironmentVariables = (scope: IEnvironmentScope): Promise<TSchemas['Variable'][]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}/variables', {
      params: { path: base(scope), query: LIST_QUERY },
    }),
  );

export const putEnvironmentVariable = (scope: IEnvironmentScope, name: string, value: string): Promise<TSchemas['Variable']> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}/variables/{variableName}', {
      params: { path: { ...base(scope), variableName: name } },
      body: { Value: value },
    }),
  );

export const removeEnvironmentVariable = (scope: IEnvironmentScope, name: string): Promise<TSchemas['Variable']> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/variables/{variableName}/remove', {
      params: { path: { ...base(scope), variableName: name } },
    }),
  );

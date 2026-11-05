import { ApiClient, request } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import {
  listEnvironmentSecrets,
  listEnvironmentVariables,
  putEnvironmentSecret,
  putEnvironmentVariable,
  removeEnvironmentSecret,
  removeEnvironmentVariable,
  type IEnvironmentScope,
} from '@/Modules/Secrets/Api/EnvironmentSecretsApi';

type TSchemas = components['schemas'];

export type TSecret = TSchemas['Secret'];
export type TSecretVisibility = TSchemas['SecretVisibility'];
export type TSecretScopeName = TSchemas['SecretScope'];
export type TVariable = TSchemas['Variable'];
export type TRegistryCredential = TSchemas['RegistryCredential'];
export type TOrganizationProject = TSchemas['Project'];

export interface ISecretScope {
  OrganizationId: string;
  ProjectId: string | null;
  EnvironmentName?: string;
}

export interface ISecretInput {
  Value: string;
  Description: string;
  AvailableToPullRequests: boolean;
  Visibility: TSecretVisibility;
  ProjectIds: string[];
}

export interface IRegistryInput {
  Username: string;
  Password: string;
}

export const ALL_SECRETS_KEY = ['secrets'] as const;

export const scopeKey = (scope: ISecretScope) =>
  [...ALL_SECRETS_KEY, scope.OrganizationId, scope.ProjectId ?? 'organization', scope.EnvironmentName ?? 'all'] as const;
export const secretsKey = (scope: ISecretScope) => [...scopeKey(scope), 'secrets'] as const;
export const variablesKey = (scope: ISecretScope) => [...scopeKey(scope), 'variables'] as const;
export const registryKey = (scope: ISecretScope) => [...scopeKey(scope), 'registry'] as const;
export const projectChoicesKey = (organizationId: string) => [...ALL_SECRETS_KEY, organizationId, 'project-choices'] as const;

const LIST_LIMIT = 500;
const PROJECT_LIMIT = 100;
const LIST_QUERY = { Page: 1, Limit: LIST_LIMIT };

const environmentOf = (scope: ISecretScope): IEnvironmentScope | null =>
  scope.ProjectId !== null && scope.EnvironmentName !== undefined
    ? { OrganizationId: scope.OrganizationId, ProjectId: scope.ProjectId, EnvironmentName: scope.EnvironmentName }
    : null;

const organizationPath = (scope: ISecretScope) => ({
  params: { path: { id: scope.OrganizationId } },
});

export const listSecrets = (scope: ISecretScope): Promise<TSecret[]> => {
  const environment = environmentOf(scope);
  if (environment !== null) return listEnvironmentSecrets(environment);
  return scope.ProjectId === null
    ? request(() =>
        ApiClient.GET('/organizations/{id}/secrets', {
          params: { path: { id: scope.OrganizationId }, query: LIST_QUERY },
        }),
      )
    : request(() =>
        ApiClient.GET('/organizations/{id}/projects/{projectId}/secrets', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
            },
            query: LIST_QUERY,
          },
        }),
      );
};

export const putSecret = (scope: ISecretScope, name: string, input: ISecretInput): Promise<TSecret> => {
  const environment = environmentOf(scope);
  if (environment !== null) {
    return putEnvironmentSecret(environment, name, { Value: input.Value, Description: input.Description });
  }
  const common = {
    Value: input.Value,
    Description: input.Description,
    AvailableToPullRequests: input.AvailableToPullRequests,
  };
  if (scope.ProjectId === null) {
    return request(() =>
      ApiClient.PUT('/organizations/{id}/secrets/{secretName}', {
        params: { path: { id: scope.OrganizationId, secretName: name } },
        body: {
          ...common,
          Visibility: input.Visibility,
          ProjectIds: input.Visibility === 'selected_projects' ? input.ProjectIds : [],
        },
      }),
    );
  }
  const projectId = scope.ProjectId;
  return request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/secrets/{secretName}', {
      params: {
        path: { id: scope.OrganizationId, projectId, secretName: name },
      },
      body: common,
    }),
  );
};

export const removeSecret = (scope: ISecretScope, name: string): Promise<TSecret> => {
  const environment = environmentOf(scope);
  if (environment !== null) return removeEnvironmentSecret(environment, name);
  return scope.ProjectId === null
    ? request(() =>
        ApiClient.POST('/organizations/{id}/secrets/{secretName}/remove', {
          params: { path: { id: scope.OrganizationId, secretName: name } },
        }),
      )
    : request(() =>
        ApiClient.POST('/organizations/{id}/projects/{projectId}/secrets/{secretName}/remove', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
              secretName: name,
            },
          },
        }),
      );
};

export const listVariables = (scope: ISecretScope): Promise<TVariable[]> => {
  const environment = environmentOf(scope);
  if (environment !== null) return listEnvironmentVariables(environment);
  return scope.ProjectId === null
    ? request(() =>
        ApiClient.GET('/organizations/{id}/variables', {
          params: { path: { id: scope.OrganizationId }, query: LIST_QUERY },
        }),
      )
    : request(() =>
        ApiClient.GET('/organizations/{id}/projects/{projectId}/variables', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
            },
            query: LIST_QUERY,
          },
        }),
      );
};

export const putVariable = (scope: ISecretScope, name: string, value: string): Promise<TVariable> => {
  const environment = environmentOf(scope);
  if (environment !== null) return putEnvironmentVariable(environment, name, value);
  return scope.ProjectId === null
    ? request(() =>
        ApiClient.PUT('/organizations/{id}/variables/{variableName}', {
          params: { path: { id: scope.OrganizationId, variableName: name } },
          body: { Value: value },
        }),
      )
    : request(() =>
        ApiClient.PUT('/organizations/{id}/projects/{projectId}/variables/{variableName}', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
              variableName: name,
            },
          },
          body: { Value: value },
        }),
      );
};

export const removeVariable = (scope: ISecretScope, name: string): Promise<TVariable> => {
  const environment = environmentOf(scope);
  if (environment !== null) return removeEnvironmentVariable(environment, name);
  return scope.ProjectId === null
    ? request(() =>
        ApiClient.POST('/organizations/{id}/variables/{variableName}/remove', {
          params: { path: { id: scope.OrganizationId, variableName: name } },
        }),
      )
    : request(() =>
        ApiClient.POST('/organizations/{id}/projects/{projectId}/variables/{variableName}/remove', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
              variableName: name,
            },
          },
        }),
      );
};

export const listRegistryCredentials = (scope: ISecretScope): Promise<TRegistryCredential[]> =>
  scope.ProjectId === null
    ? request(() =>
        ApiClient.GET('/organizations/{id}/registry-credentials', {
          params: { path: { id: scope.OrganizationId }, query: LIST_QUERY },
        }),
      )
    : request(() =>
        ApiClient.GET('/organizations/{id}/projects/{projectId}/registry-credentials', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
            },
            query: LIST_QUERY,
          },
        }),
      );

export const createRegistryCredential = (scope: ISecretScope, host: string, input: IRegistryInput): Promise<TRegistryCredential> =>
  scope.ProjectId === null
    ? request(() =>
        ApiClient.POST('/organizations/{id}/registry-credentials', {
          ...organizationPath(scope),
          body: {
            Host: host,
            Username: input.Username,
            Password: input.Password,
          },
        }),
      )
    : request(() =>
        ApiClient.POST('/organizations/{id}/projects/{projectId}/registry-credentials', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
            },
          },
          body: {
            Host: host,
            Username: input.Username,
            Password: input.Password,
          },
        }),
      );

export const updateRegistryCredential = (scope: ISecretScope, credentialId: string, input: IRegistryInput): Promise<TRegistryCredential> =>
  scope.ProjectId === null
    ? request(() =>
        ApiClient.PUT('/organizations/{id}/registry-credentials/{credentialId}', {
          params: { path: { id: scope.OrganizationId, credentialId } },
          body: { Username: input.Username, Password: input.Password },
        }),
      )
    : request(() =>
        ApiClient.PUT('/organizations/{id}/projects/{projectId}/registry-credentials/{credentialId}', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
              credentialId,
            },
          },
          body: { Username: input.Username, Password: input.Password },
        }),
      );

export const removeRegistryCredential = (scope: ISecretScope, credentialId: string): Promise<TRegistryCredential> =>
  scope.ProjectId === null
    ? request(() =>
        ApiClient.POST('/organizations/{id}/registry-credentials/{credentialId}/remove', {
          params: { path: { id: scope.OrganizationId, credentialId } },
        }),
      )
    : request(() =>
        ApiClient.POST('/organizations/{id}/projects/{projectId}/registry-credentials/{credentialId}/remove', {
          params: {
            path: {
              id: scope.OrganizationId,
              projectId: scope.ProjectId ?? '',
              credentialId,
            },
          },
        }),
      );

export const listProjectChoices = (organizationId: string): Promise<TOrganizationProject[]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects', {
      params: {
        path: { id: organizationId },
        query: { Page: 1, Limit: PROJECT_LIMIT, State: 'active' },
      },
    }),
  );

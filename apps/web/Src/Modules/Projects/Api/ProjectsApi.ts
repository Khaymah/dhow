import { ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';

type TSchemas = components['schemas'];

export type TProject = TSchemas['Project'];
export type TProjectState = TSchemas['ProjectState'];
export type TForkPolicy = TSchemas['ForkPolicy'];
export type TCreateProject = TSchemas['CreateProjectRequest'];
export type TUpdateProject = TSchemas['UpdateProjectRequest'];
export type TRepository = TSchemas['Repository'];
export type TRepositoryWithSecret = TSchemas['RepositoryWithSecret'];
export type TLinkRepository = TSchemas['LinkRepositoryRequest'];

export const PROJECTS_PAGE_SIZE = 20;

export const projectsKey = (organizationId: string) => ['projects', organizationId] as const;
export const projectListKey = (organizationId: string) => [...projectsKey(organizationId), 'list'] as const;
export const projectKey = (organizationId: string, projectId: string) => [...projectsKey(organizationId), 'detail', projectId] as const;
export const repositoryKey = (organizationId: string, projectId: string) =>
  [...projectsKey(organizationId), 'repository', projectId] as const;

const projectPath = (id: string, projectId: string) => ({ params: { path: { id, projectId } } });

export const listProjects = (organizationId: string, page: number, state?: TProjectState): Promise<IPage<TProject>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects', {
      params: { path: { id: organizationId }, query: { Page: page, Limit: PROJECTS_PAGE_SIZE, State: state } },
    }),
  );

export const getProject = (organizationId: string, projectId: string): Promise<TProject> =>
  request(() => ApiClient.GET('/organizations/{id}/projects/{projectId}', projectPath(organizationId, projectId)));

export const createProject = (organizationId: string, body: TCreateProject): Promise<TProject> =>
  request(() => ApiClient.POST('/organizations/{id}/projects', { params: { path: { id: organizationId } }, body }));

export const updateProject = (organizationId: string, projectId: string, body: TUpdateProject): Promise<TProject> =>
  request(() => ApiClient.PATCH('/organizations/{id}/projects/{projectId}', { ...projectPath(organizationId, projectId), body }));

export const archiveProject = (organizationId: string, projectId: string): Promise<TProject> =>
  request(() => ApiClient.POST('/organizations/{id}/projects/{projectId}/archive', projectPath(organizationId, projectId)));

export const getRepository = (organizationId: string, projectId: string): Promise<TRepository> =>
  request(() => ApiClient.GET('/organizations/{id}/projects/{projectId}/repository', projectPath(organizationId, projectId)));

export const linkRepository = (organizationId: string, projectId: string, body: TLinkRepository): Promise<TRepositoryWithSecret> =>
  request(() => ApiClient.PUT('/organizations/{id}/projects/{projectId}/repository', { ...projectPath(organizationId, projectId), body }));

export const unlinkRepository = (organizationId: string, projectId: string): Promise<TRepository> =>
  request(() => ApiClient.DELETE('/organizations/{id}/projects/{projectId}/repository', projectPath(organizationId, projectId)));

export const confirmHostKey = (organizationId: string, projectId: string, fingerprint: string): Promise<TRepository> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/repository/confirm-host-key', {
      ...projectPath(organizationId, projectId),
      body: { Fingerprint: fingerprint },
    }),
  );

export const rotateDeployKey = (organizationId: string, projectId: string): Promise<TRepository> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/repository/rotate-deploy-key', projectPath(organizationId, projectId)),
  );

export const confirmDeployKey = (organizationId: string, projectId: string): Promise<TRepository> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/repository/confirm-deploy-key', projectPath(organizationId, projectId)),
  );

export const rotateWebhookSecret = (organizationId: string, projectId: string): Promise<TRepositoryWithSecret> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/repository/rotate-webhook-secret', projectPath(organizationId, projectId)),
  );

import { keepPreviousData, useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IPage } from '@/Core/Api/Client';
import {
  archiveProject,
  getProject,
  listProjects,
  projectKey,
  projectListKey,
  updateProject,
  type TProject,
  type TProjectState,
  type TUpdateProject,
} from '@/Modules/Projects/Api/ProjectsApi';

export const useProjects = (organizationId: string, page: number, state?: TProjectState): UseQueryResult<IPage<TProject>, ApiError> =>
  useQuery<IPage<TProject>, ApiError>({
    queryKey: [...projectListKey(organizationId), page, state ?? 'all'],
    queryFn: () => listProjects(organizationId, page, state),
    placeholderData: keepPreviousData,
  });

export const useProject = (organizationId: string, projectId: string): UseQueryResult<TProject, ApiError> =>
  useQuery<TProject, ApiError>({
    queryKey: projectKey(organizationId, projectId),
    queryFn: () => getProject(organizationId, projectId),
    enabled: organizationId !== '' && projectId !== '',
  });

const useProjectSaved = (organizationId: string) => {
  const client = useQueryClient();
  return (project: TProject) => {
    client.setQueryData(projectKey(organizationId, project.Id), project);
    void client.invalidateQueries({ queryKey: projectListKey(organizationId) });
  };
};

export const useUpdateProject = (organizationId: string, projectId: string): UseMutationResult<TProject, ApiError, TUpdateProject> => {
  const saved = useProjectSaved(organizationId);
  return useMutation<TProject, ApiError, TUpdateProject>({
    mutationFn: (body) => updateProject(organizationId, projectId, body),
    onSuccess: saved,
  });
};

export const useArchiveProject = (organizationId: string, projectId: string): UseMutationResult<TProject, ApiError, void> => {
  const saved = useProjectSaved(organizationId);
  return useMutation<TProject, ApiError>({
    mutationFn: () => archiveProject(organizationId, projectId),
    onSuccess: saved,
  });
};

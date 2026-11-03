import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import { isApiError, type ApiError } from '@/Core/Api/ApiError';
import {
  confirmDeployKey,
  confirmHostKey,
  getRepository,
  repositoryKey,
  rotateDeployKey,
  unlinkRepository,
  type TRepository,
} from '@/Modules/Projects/Api/ProjectsApi';
import { manualInputsKey } from '@/Modules/Projects/Api/RunsApi';

export const NOT_LINKED = 'REPOSITORY_NOT_LINKED';
const PENDING_REFRESH_MS = 15_000;

export const isNotLinked = (error: unknown): boolean => isApiError(error) && error.Code === NOT_LINKED;

export const useRepository = (organizationId: string, projectId: string): UseQueryResult<TRepository, ApiError> =>
  useQuery<TRepository, ApiError>({
    queryKey: repositoryKey(organizationId, projectId),
    queryFn: () => getRepository(organizationId, projectId),
    enabled: organizationId !== '' && projectId !== '',
    retry: false,
    refetchInterval: (query) => (query.state.data?.LastDeliveryAt === undefined && query.state.data !== undefined ? PENDING_REFRESH_MS : false),
  });

export const useRepositorySaved = (organizationId: string, projectId: string) => {
  const client = useQueryClient();
  return (repository: TRepository | null) => {
    if (repository === null) void client.resetQueries({ queryKey: repositoryKey(organizationId, projectId) });
    else {
      client.setQueryData(repositoryKey(organizationId, projectId), repository);
      void client.invalidateQueries({ queryKey: repositoryKey(organizationId, projectId) });
    }
    void client.invalidateQueries({ queryKey: manualInputsKey(organizationId, projectId) });
  };
};

type TRepositoryAction = 'confirm-deploy-key' | 'rotate-deploy-key';

export const useRepositoryAction = (
  organizationId: string,
  projectId: string,
  action: TRepositoryAction,
): UseMutationResult<TRepository, ApiError, void> => {
  const saved = useRepositorySaved(organizationId, projectId);
  return useMutation<TRepository, ApiError>({
    mutationFn: () => (action === 'rotate-deploy-key' ? rotateDeployKey(organizationId, projectId) : confirmDeployKey(organizationId, projectId)),
    onSuccess: saved,
  });
};

export const useConfirmHostKey = (organizationId: string, projectId: string): UseMutationResult<TRepository, ApiError, string> => {
  const saved = useRepositorySaved(organizationId, projectId);
  return useMutation<TRepository, ApiError, string>({
    mutationFn: (fingerprint) => confirmHostKey(organizationId, projectId, fingerprint),
    onSuccess: saved,
  });
};

export const useUnlinkRepository = (organizationId: string, projectId: string): UseMutationResult<TRepository, ApiError, void> => {
  const saved = useRepositorySaved(organizationId, projectId);
  return useMutation<TRepository, ApiError>({
    mutationFn: () => unlinkRepository(organizationId, projectId),
    onSuccess: () => {
      saved(null);
    },
  });
};

import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IPage } from '@/Core/Api/Client';
import {
  getTargetCheck,
  listTargetReleases,
  listTargets,
  targetCheckKey,
  targetReleasesKey,
  targetsKey,
  type TTarget,
  type TTargetCheck,
  type TTargetReleases,
} from '@/Modules/Environments/Api/TargetsApi';
import { checkIsPending, CHECK_POLL_MS } from '@/Modules/Environments/Types/TargetState';

export const useTargets = (organizationId: string, projectId: string, environmentName: string): UseQueryResult<IPage<TTarget>, ApiError> =>
  useQuery<IPage<TTarget>, ApiError>({
    queryKey: targetsKey(organizationId, projectId, environmentName),
    queryFn: () => listTargets(organizationId, projectId, environmentName),
  });

export const useTargetCheck = (
  organizationId: string,
  projectId: string,
  environmentName: string,
  name: string,
  since: number | null,
): UseQueryResult<TTargetCheck, ApiError> =>
  useQuery<TTargetCheck, ApiError>({
    queryKey: targetCheckKey(organizationId, projectId, environmentName, name),
    queryFn: () => getTargetCheck(organizationId, projectId, environmentName, name),
    refetchInterval: (query) => (checkIsPending(query.state.data, since, Date.now()) ? CHECK_POLL_MS : false),
  });

export const useTargetReleases = (
  organizationId: string,
  projectId: string,
  environmentName: string,
  name: string,
): UseQueryResult<TTargetReleases, ApiError> =>
  useQuery<TTargetReleases, ApiError>({
    queryKey: targetReleasesKey(organizationId, projectId, environmentName, name),
    queryFn: () => listTargetReleases(organizationId, projectId, environmentName, name),
  });

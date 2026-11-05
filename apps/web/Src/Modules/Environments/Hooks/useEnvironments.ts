import { keepPreviousData, useQuery, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IPage } from '@/Core/Api/Client';
import {
  DEPLOYED_SCAN_SIZE,
  DEPLOYMENTS_PAGE_SIZE,
  deployedKey,
  deploymentsKey,
  environmentKey,
  environmentListKey,
  getEnvironment,
  listDeployments,
  listEnvironments,
  listReviewerMembers,
  listReviewerTeams,
  reviewerChoicesKey,
  type TDeployment,
  type TEnvironment,
  type TReviewerMember,
  type TReviewerTeam,
} from '@/Modules/Environments/Api/EnvironmentsApi';

export const useEnvironments = (organizationId: string, projectId: string): UseQueryResult<TEnvironment[], ApiError> =>
  useQuery<TEnvironment[], ApiError>({
    queryKey: environmentListKey(organizationId, projectId),
    queryFn: () => listEnvironments(organizationId, projectId),
  });

export const useEnvironment = (organizationId: string, projectId: string, name: string): UseQueryResult<TEnvironment, ApiError> =>
  useQuery<TEnvironment, ApiError>({
    queryKey: environmentKey(organizationId, projectId, name),
    queryFn: () => getEnvironment(organizationId, projectId, name),
  });

export const useDeployments = (
  organizationId: string,
  projectId: string,
  name: string,
  page: number,
): UseQueryResult<IPage<TDeployment>, ApiError> =>
  useQuery<IPage<TDeployment>, ApiError>({
    queryKey: [...deploymentsKey(organizationId, projectId, name), page],
    queryFn: () => listDeployments(organizationId, projectId, name, page, DEPLOYMENTS_PAGE_SIZE),
    placeholderData: keepPreviousData,
  });

export const useDeployedNow = (organizationId: string, projectId: string, name: string): UseQueryResult<TDeployment | null, ApiError> =>
  useQuery<TDeployment | null, ApiError>({
    queryKey: deployedKey(organizationId, projectId, name),
    queryFn: async () => {
      const page = await listDeployments(organizationId, projectId, name, 1, DEPLOYED_SCAN_SIZE);
      return page.Items.find((deployment) => deployment.State === 'succeeded') ?? null;
    },
  });

export const useReviewerMembers = (organizationId: string, enabled: boolean): UseQueryResult<TReviewerMember[], ApiError> =>
  useQuery<TReviewerMember[], ApiError>({
    queryKey: [...reviewerChoicesKey(organizationId), 'members'],
    queryFn: () => listReviewerMembers(organizationId),
    enabled,
  });

export const useReviewerTeams = (organizationId: string, enabled: boolean): UseQueryResult<TReviewerTeam[], ApiError> =>
  useQuery<TReviewerTeam[], ApiError>({
    queryKey: [...reviewerChoicesKey(organizationId), 'teams'],
    queryFn: () => listReviewerTeams(organizationId),
    enabled,
  });

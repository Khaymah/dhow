import { useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import {
  approvalsKey,
  bypassKey,
  listMyApprovals,
  listRunApprovals,
  myApprovalsKey,
  readBypassAllowed,
  runApprovalsKey,
  type TApproval,
} from '@/Modules/Approvals/Api/ApprovalsApi';

export const RUN_APPROVALS_REFRESH_MS = 4_000;
export const MY_APPROVALS_REFRESH_MS = 15_000;

export const useRunApprovals = (
  organizationId: string,
  projectId: string,
  runId: string,
  enabled: boolean,
  live: boolean,
): UseQueryResult<TApproval[], ApiError> =>
  useQuery<TApproval[], ApiError>({
    queryKey: runApprovalsKey(organizationId, projectId, runId),
    queryFn: () => listRunApprovals(organizationId, projectId, runId),
    enabled,
    refetchInterval: live ? RUN_APPROVALS_REFRESH_MS : false,
  });

export const useMyApprovals = (organizationId: string, enabled = true): UseQueryResult<TApproval[], ApiError> =>
  useQuery<TApproval[], ApiError>({
    queryKey: myApprovalsKey(organizationId),
    queryFn: () => listMyApprovals(organizationId),
    enabled,
    refetchInterval: MY_APPROVALS_REFRESH_MS,
  });

export const useBypassAllowed = (
  organizationId: string,
  projectId: string,
  environment: string,
  enabled: boolean,
): UseQueryResult<boolean, ApiError> =>
  useQuery<boolean, ApiError>({
    queryKey: bypassKey(organizationId, projectId, environment),
    queryFn: () => readBypassAllowed(organizationId, projectId, environment),
    enabled,
    retry: false,
  });

export const useApprovalsRefresh = (organizationId: string): (() => void) => {
  const client = useQueryClient();
  return () => {
    void client.invalidateQueries({ queryKey: approvalsKey(organizationId) });
  };
};

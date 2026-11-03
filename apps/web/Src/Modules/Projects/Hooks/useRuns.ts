import { keepPreviousData, useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IPage } from '@/Core/Api/Client';
import {
  approveRun,
  artifactsKey,
  cancelRun,
  getManualInputs,
  getRun,
  listArtifacts,
  listRuns,
  manualInputsKey,
  rerunFailed,
  runKey,
  runListKey,
  runsKey,
  type TArtifact,
  type TManualInputs,
  type TRun,
  type TRunDetail,
  type TRunState,
} from '@/Modules/Projects/Api/RunsApi';
import { isFinalRun } from '@/Modules/Projects/Types/RunStates';

export const LIST_REFRESH_MS = 10_000;
export const DETAIL_REFRESH_MS = 4_000;

const anyActive = (page: IPage<TRun> | undefined): boolean => page?.Items.some((run) => !isFinalRun(run.State)) ?? false;

export const useRuns = (
  organizationId: string,
  projectId: string,
  page: number,
  states: readonly TRunState[],
): UseQueryResult<IPage<TRun>, ApiError> =>
  useQuery<IPage<TRun>, ApiError>({
    queryKey: [...runListKey(organizationId, projectId), page, states],
    queryFn: () => listRuns(organizationId, projectId, page, states),
    placeholderData: keepPreviousData,
    refetchInterval: (query) => (anyActive(query.state.data) ? LIST_REFRESH_MS : false),
  });

export const useRun = (organizationId: string, projectId: string, runId: string): UseQueryResult<TRunDetail, ApiError> =>
  useQuery<TRunDetail, ApiError>({
    queryKey: runKey(organizationId, projectId, runId),
    queryFn: () => getRun(organizationId, projectId, runId),
    refetchInterval: (query) => {
      const run = query.state.data?.Run;
      return run !== undefined && !isFinalRun(run.State) ? DETAIL_REFRESH_MS : false;
    },
  });

export const useArtifacts = (
  organizationId: string,
  projectId: string,
  runId: string,
  final: boolean,
): UseQueryResult<TArtifact[], ApiError> =>
  useQuery<TArtifact[], ApiError>({
    queryKey: [...artifactsKey(organizationId, projectId, runId), final],
    queryFn: () => listArtifacts(organizationId, projectId, runId),
  });

export const useManualInputs = (organizationId: string, projectId: string, enabled = true): UseQueryResult<TManualInputs, ApiError> =>
  useQuery<TManualInputs, ApiError>({
    queryKey: manualInputsKey(organizationId, projectId),
    queryFn: () => getManualInputs(organizationId, projectId),
    enabled,
    retry: false,
  });

export type TRunAction = 'cancel' | 'rerun' | 'approve';

const ACTIONS: Record<TRunAction, (organizationId: string, projectId: string, runId: string) => Promise<TRunDetail>> = {
  cancel: cancelRun,
  rerun: rerunFailed,
  approve: approveRun,
};

export const useRunAction = (
  organizationId: string,
  projectId: string,
  runId: string,
  action: TRunAction,
): UseMutationResult<TRunDetail, ApiError, void> => {
  const client = useQueryClient();
  return useMutation<TRunDetail, ApiError>({
    mutationFn: () => ACTIONS[action](organizationId, projectId, runId),
    onSuccess: (detail) => {
      client.setQueryData(runKey(organizationId, projectId, runId), detail);
      void client.invalidateQueries({ queryKey: runsKey(organizationId, projectId) });
    },
  });
};

import { keepPreviousData, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IPage } from '@/Core/Api/Client';
import {
  ingestedKey,
  ingestKey,
  ingestTokensKey,
  listIngestedArtifacts,
  listIngestTokens,
  type TIngestedArtifact,
  type TIngestToken,
} from '@/Modules/Ingest/Api/IngestApi';

export const useIngestTokens = (
  organizationId: string,
  projectId: string,
  page: number,
): UseQueryResult<IPage<TIngestToken>, ApiError> =>
  useQuery<IPage<TIngestToken>, ApiError>({
    queryKey: [...ingestTokensKey(organizationId, projectId), page],
    queryFn: () => listIngestTokens(organizationId, projectId, page),
    placeholderData: keepPreviousData,
  });

export const useIngestedArtifacts = (
  organizationId: string,
  projectId: string,
  page: number,
): UseQueryResult<IPage<TIngestedArtifact>, ApiError> =>
  useQuery<IPage<TIngestedArtifact>, ApiError>({
    queryKey: [...ingestedKey(organizationId, projectId), page],
    queryFn: () => listIngestedArtifacts(organizationId, projectId, page),
    placeholderData: keepPreviousData,
  });

export const useIngestRefresh = (organizationId: string, projectId: string) => {
  const client = useQueryClient();
  return {
    Changed: () => client.invalidateQueries({ queryKey: ingestKey(organizationId, projectId) }),
  };
};

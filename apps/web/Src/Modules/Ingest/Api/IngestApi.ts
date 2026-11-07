import { ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';

type TSchemas = components['schemas'];

export type TIngestToken = TSchemas['IngestToken'];
export type TIngestRefPattern = TSchemas['IngestRefPattern'];
export type TIngestRefKind = TSchemas['IngestRefKind'];
export type TCreatedIngestToken = TSchemas['CreatedIngestToken'];
export type TIngestedArtifact = TSchemas['IngestedArtifact'];

export const INGEST_PAGE_SIZE = 20;

export const ingestKey = (organizationId: string, projectId: string) => ['ingest', organizationId, projectId] as const;
export const ingestTokensKey = (organizationId: string, projectId: string) => [...ingestKey(organizationId, projectId), 'tokens'] as const;
export const ingestedKey = (organizationId: string, projectId: string) => [...ingestKey(organizationId, projectId), 'received'] as const;

export interface ICreateIngestToken {
  Name: string;
  RefPatterns: TIngestRefPattern[];
  ExpiresInDays: number;
  AcknowledgeProtectedReach: boolean;
}

export const listIngestTokens = (organizationId: string, projectId: string, page: number): Promise<IPage<TIngestToken>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/ingest-tokens', {
      params: { path: { id: organizationId, projectId }, query: { Page: page, Limit: INGEST_PAGE_SIZE } },
    }),
  );

export const createIngestToken = (
  organizationId: string,
  projectId: string,
  input: ICreateIngestToken,
  factor: TCurrentFactor,
): Promise<TCreatedIngestToken> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/ingest-tokens', {
      params: { path: { id: organizationId, projectId } },
      body: { ...input, CurrentFactor: factor },
    }),
  );

export const revokeIngestToken = (organizationId: string, projectId: string, tokenId: string, factor: TCurrentFactor): Promise<TIngestToken> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/ingest-tokens/{ingestTokenId}/revoke', {
      params: { path: { id: organizationId, projectId, ingestTokenId: tokenId } },
      body: { CurrentFactor: factor },
    }),
  );

export const acknowledgeIngestTokenReach = (
  organizationId: string,
  projectId: string,
  tokenId: string,
  environments: readonly string[],
  factor: TCurrentFactor,
): Promise<TIngestToken> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/ingest-tokens/{ingestTokenId}/acknowledge-reach', {
      params: { path: { id: organizationId, projectId, ingestTokenId: tokenId } },
      body: { CurrentFactor: factor, Environments: [...environments] },
    }),
  );

export const listIngestedArtifacts = (organizationId: string, projectId: string, page: number): Promise<IPage<TIngestedArtifact>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/ingested-artifacts', {
      params: { path: { id: organizationId, projectId }, query: { Page: page, Limit: INGEST_PAGE_SIZE } },
    }),
  );

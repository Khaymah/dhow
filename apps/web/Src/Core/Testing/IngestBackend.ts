import type { TRouteHandler } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';

export const TOKEN_ID = '0198a000-0000-7000-8000-0000000009a1';
export const OTHER_TOKEN_ID = '0198a000-0000-7000-8000-0000000009a2';
export const FILE_ID = '0198a000-0000-7000-8000-0000000009b1';
export const FILE_RUN_ID = '0198a000-0000-7000-8000-000000000401';
export const FILE_DIGEST = '3b7f2c1d9e4a5b6c7d8e9f00112233445566778899aabbccddeeff0011223344';
export const NEW_TOKEN_VALUE = ['dhow', 'ing', 'zQ8vK2mN5pX7cR4tY1wB6hJ3dF9gL0aSeU2iOkM'].join('_');

export interface IIngestWrite {
  Method: string;
  Path: string;
  Body: Record<string, unknown> | null;
}

export interface IIngestState {
  tokens: Record<string, unknown>[];
  files: Record<string, unknown>[];
  protectedReach: string[];
  writes: IIngestWrite[];
  failCreate: Response | null;
  failTokens: Response | null;
  holdTokens: Promise<void> | null;
  emptyReach: boolean;
}

export const ingestTokenFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: TOKEN_ID,
  ProjectId: '0198a000-0000-7000-8000-000000000301',
  Name: 'github-main',
  Prefix: 'dhow_ing_AbCd',
  RefPatterns: [{ Kind: 'branch', Pattern: 'main' }],
  Status: 'active',
  CreatedAt: '2026-11-01T08:00:00Z',
  ExpiresAt: '2999-01-01T00:00:00Z',
  LastUsedAt: '2026-11-05T08:00:00Z',
  CreatedByUserId: '0198a000-0000-7000-8000-0000000000b1',
  Reaches: ['staging'],
  CreatorAllowed: true,
  UnacknowledgedReach: [],
  ...over,
});

export const ingestFileFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: FILE_ID,
  Name: 'web',
  Digest: FILE_DIGEST,
  SizeBytes: 2 * 1024 * 1024,
  Ref: 'refs/heads/main',
  RefKind: 'branch',
  Sha: '0123456789abcdef0123456789abcdef01234567',
  Provenance: { Provider: 'github-actions', Actor: 'octocat' },
  ProvenanceVerified: false,
  TokenName: 'github-main',
  RunId: FILE_RUN_ID,
  RunNumber: 12,
  RunState: 'succeeded',
  CreatedAt: '2026-11-05T08:00:00Z',
  ExpiresAt: '2999-01-01T00:00:00Z',
  ...over,
});

export const newIngestState = (): IIngestState => ({
  tokens: [ingestTokenFixture(), ingestTokenFixture({ Id: OTHER_TOKEN_ID, Name: 'old-ci', Status: 'revoked', Reaches: [], LastUsedAt: undefined })],
  files: [ingestFileFixture()],
  protectedReach: [],
  writes: [],
  failCreate: null,
  failTokens: null,
  holdTokens: null,
  emptyReach: false,
});

const PATH = /^\/api\/v1\/organizations\/[^/]+\/projects\/[^/]+\/(ingest-tokens(?:\/([^/]+)\/(revoke|acknowledge-reach))?|ingested-artifacts)$/;

const pageOf = (items: readonly unknown[], url: URL): Response => {
  const page = Number(url.searchParams.get('Page') ?? '1');
  const limit = Number(url.searchParams.get('Limit') ?? '20');
  return envelope({ Success: true, Data: items.slice((page - 1) * limit, page * limit), Meta: { Page: page, Limit: limit, Total: items.length } });
};

const bodyOf = async (request: Request): Promise<Record<string, unknown> | null> => {
  try {
    return (await request.clone().json()) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const unconfirmed = (names: readonly string[]): Response =>
  envelope(
    {
      Success: false,
      Error: {
        Code: 'INGEST_TOKEN_REACH_UNCONFIRMED',
        Message: 'تحتاج إلى تأكيد',
        Details: [{ Field: 'AcknowledgeProtectedReach', Message: 'x' }, ...names.map((name) => ({ Field: 'ProtectedReach', Message: name }))],
      },
    },
    422,
  );

export const ingestRoute =
  (state: IIngestState): TRouteHandler =>
  (request, url) => {
    const match = PATH.exec(url.pathname);
    if (!match) return undefined;
    const [, kind, revokeId, action] = match;
    if (request.method === 'GET' && kind === 'ingest-tokens' && state.failTokens) return state.failTokens.clone();
    if (request.method === 'GET' && kind === 'ingest-tokens' && state.holdTokens) return state.holdTokens.then(() => pageOf(state.tokens, url));
    if (request.method === 'GET') return pageOf(kind === 'ingested-artifacts' ? state.files : state.tokens, url);
    return bodyOf(request).then((body) => {
      state.writes.push({ Method: request.method, Path: url.pathname.replace(/^\/api\/v1/, ''), Body: body });
      if (revokeId !== undefined) {
        const change = action === 'revoke' ? { Status: 'revoked' } : { UnacknowledgedReach: [] };
        state.tokens = state.tokens.map((token) => (token.Id === revokeId ? { ...token, ...change } : token));
        return envelope({ Success: true, Data: state.tokens.find((token) => token.Id === revokeId) }, 200);
      }
      if (state.failCreate) return state.failCreate.clone();
      if (state.emptyReach && body?.AcknowledgeProtectedReach !== true) return unconfirmed([]);
      if (state.protectedReach.length > 0 && body?.AcknowledgeProtectedReach !== true) return unconfirmed(state.protectedReach);
      const token = ingestTokenFixture({
        Id: '0198a000-0000-7000-8000-0000000009a9',
        Name: body?.Name,
        RefPatterns: body?.RefPatterns,
        Reaches: state.protectedReach,
        LastUsedAt: undefined,
      });
      state.tokens = [token, ...state.tokens];
      return envelope({ Success: true, Data: { Token: token, Value: NEW_TOKEN_VALUE } }, 201);
    });
  };

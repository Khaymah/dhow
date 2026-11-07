import type { TRouteHandler } from '@/Core/Testing/Backend';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';

export const BOARD_ENVIRONMENT_ID = '0198a000-0000-7000-8000-000000000a01';
export const BOARD_TARGET_ID = '0198a000-0000-7000-8000-000000000d01';
export const BOARD_DIGEST = 'ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12';

export type TBoardFixture = Record<string, unknown>;

export const boardTargetFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: BOARD_TARGET_ID,
  Name: 'web',
  Kind: 'ssh',
  Hosts: 2,
  CurrentReleaseId: 'rel-41',
  CurrentDigest: BOARD_DIGEST,
  Last: { Id: '0198a000-0000-7000-8000-000000000e01', ReleaseId: 'rel-41', Kind: 'deploy', State: 'succeeded', StartedAt: '2026-11-07T09:00:00Z', FinishedAt: '2026-11-07T09:01:00Z' },
  Partial: false,
  AutoRolledBack: false,
  RollbackFailed: false,
  ...over,
});

export const boardEnvironmentFixture = (over: Record<string, unknown> = {}, targets: Record<string, unknown>[] = [boardTargetFixture()]): Record<string, unknown> => ({
  Id: BOARD_ENVIRONMENT_ID,
  Name: 'production',
  Paused: false,
  PendingApprovals: 0,
  Targets: targets,
  ...over,
});

export const boardFixture = (environments: Record<string, unknown>[] = [boardEnvironmentFixture()], truncated = false): TBoardFixture => ({
  Environments: environments,
  Truncated: truncated,
});

export const runningFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  ReleaseId: 'rel-42',
  Hosts: 6,
  HostsDone: 2,
  BatchSize: 2,
  Batch: 2,
  Batches: 3,
  ...over,
});

export interface IBoardState {
  board: TBoardFixture;
  etag: string;
  conditional: (string | null)[];
  hits: number;
  fail: boolean;
}

export const newBoardState = (board: TBoardFixture = boardFixture()): IBoardState => ({ board, etag: '"v1"', conditional: [], hits: 0, fail: false });

export const BOARD_PATH = /^\/api\/v1\/organizations\/[^/]+\/projects\/[^/]+\/deploy-board$/;

export const boardRoute =
  (state: IBoardState): TRouteHandler =>
  (request, url) => {
    if (!BOARD_PATH.test(url.pathname) || request.method !== 'GET') return undefined;
    state.hits += 1;
    const sent = request.headers.get('If-None-Match');
    state.conditional.push(sent);
    if (state.fail) return new Response(JSON.stringify({ Success: false, Error: { Code: 'INTERNAL', Message: 'x' } }), { status: 500, headers: { 'Content-Type': 'application/json' } });
    if (sent === state.etag) return new Response(null, { status: 304 });
    return new Response(JSON.stringify({ Success: true, Data: state.board }), {
      status: 200,
      headers: { 'Content-Type': 'application/json', ETag: state.etag },
    });
  };

export const boardUrl = (): string => `/projects/${PROJECT_ID}/deploy`;

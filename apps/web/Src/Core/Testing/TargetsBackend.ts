import { backendFailure, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';

export const TARGET_NAME = 'web';
export const HOST_ONE = '0198a000-0000-7000-8000-0000000007a1';
export const HOST_TWO = '0198a000-0000-7000-8000-0000000007a2';
export const HOST_THREE = '0198a000-0000-7000-8000-0000000007a3';
export const CHECK_RUN_ID = '0198a000-0000-7000-8000-000000000b01';
export const ROLLBACK_RUN_ID = '0198a000-0000-7000-8000-000000000b02';

export interface ITargetWrite {
  Method: string;
  Path: string;
  Body: Record<string, unknown> | null;
}

export interface ITargetsState {
  targets: Record<string, unknown>[];
  check: Record<string, unknown>;
  checkAfter: Record<string, unknown> | null;
  releases: Record<string, unknown>;
  writes: ITargetWrite[];
  failRollback: Response | null;
  failCheck: Response | null;
}

export const targetFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: '0198a000-0000-7000-8000-0000000007b1',
  EnvironmentId: '0198a000-0000-7000-8000-000000000a01',
  Name: TARGET_NAME,
  Kind: 'ssh',
  Username: 'deploy',
  DeployPath: '/srv/app',
  KeepReleases: 5,
  Settings: { BatchSize: 1, BatchPauseSeconds: 0 },
  AllowedImages: [],
  Hosts: [HOST_ONE, HOST_TWO, HOST_THREE].map((id, index) => ({
    Id: id,
    Position: index + 1,
    Host: `web-${String(index + 1)}.example.com`,
    Port: 22,
    KeyPinned: true,
  })),
  Ready: true,
  NotReady: [],
  Version: 4,
  CreatedAt: '2026-11-01T08:00:00Z',
  UpdatedAt: '2026-11-02T08:00:00Z',
  ...over,
});

export const hostCheckFixture = (id: string, position: number, over: Record<string, unknown> = {}): Record<string, unknown> => ({
  HostId: id,
  Position: position,
  Host: `web-${String(position)}.example.com`,
  Port: 22,
  Checked: true,
  Reason: 'ok',
  CurrentReleaseId: '12-aaaaaaaa-1',
  VerifiedReleaseId: '12-aaaaaaaa-1',
  FreeBytes: 5 * 1024 * 1024 * 1024,
  Locked: false,
  Repaired: false,
  CheckedAt: '2026-11-05T08:00:00Z',
  ...over,
});

export const neverChecked = (): Record<string, unknown> => ({
  Hosts: [HOST_ONE, HOST_TWO, HOST_THREE].map((id, index) => ({
    HostId: id,
    Position: index + 1,
    Host: `web-${String(index + 1)}.example.com`,
    Port: 22,
    Checked: false,
    Locked: false,
    Repaired: false,
  })),
});

export const releaseFixture = (releaseId: string, over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: `0198a000-0000-7000-8000-00000000c${releaseId.slice(0, 3).replace(/\D/g, '').padStart(3, '0')}`,
  ReleaseId: releaseId,
  Kind: 'deploy',
  State: 'succeeded',
  StartedAt: '2026-11-04T08:00:00Z',
  FinishedAt: '2026-11-04T08:02:00Z',
  RollbackTarget: true,
  ArtifactKept: true,
  Hosts: [HOST_ONE, HOST_TWO, HOST_THREE].map((id, index) => ({ HostId: id, Position: index + 1, State: 'verified' })),
  ...over,
});

export const newTargetsState = (): ITargetsState => ({
  targets: [targetFixture()],
  check: neverChecked(),
  checkAfter: null,
  releases: {
    CurrentReleaseId: '12-aaaaaaaa-1',
    Partial: false,
    Releases: [releaseFixture('12-aaaaaaaa-1'), releaseFixture('11-bbbbbbbb-1', { StartedAt: '2026-11-03T08:00:00Z' })],
  },
  writes: [],
  failRollback: null,
  failCheck: null,
});

const PATH = /^\/api\/v1\/organizations\/[^/]+\/projects\/[^/]+\/environments\/[^/]+\/targets(?:\/([^/]+)(?:\/(check|releases|rollback))?)?$/;

const ok = (data: unknown, status = 200): Response => envelope({ Success: true, Data: data }, status);

const bodyOf = async (request: Request): Promise<Record<string, unknown> | null> => {
  try {
    return (await request.clone().json()) as Record<string, unknown>;
  } catch {
    return null;
  }
};

export const targetsRoute =
  (state: ITargetsState): TRouteHandler =>
  (request, url) => {
    const match = PATH.exec(url.pathname);
    if (!match) return undefined;
    const [, name, action] = match;
    if (request.method === 'GET') {
      if (name === undefined) return envelope({ Success: true, Data: state.targets, Meta: { Page: 1, Limit: 100, Total: state.targets.length } });
      if (action === undefined) {
        const found = state.targets.find((target) => target.Name === name);
        return found === undefined ? backendFailure('NOT_FOUND', 'x', 404) : ok(found);
      }
      return ok(action === 'check' ? state.check : state.releases);
    }
    return bodyOf(request).then((body) => {
      state.writes.push({ Method: request.method, Path: url.pathname.replace(/^\/api\/v1/, ''), Body: body });
      if (action === 'check') {
        if (state.failCheck) return state.failCheck.clone();
        if (state.checkAfter !== null) state.check = state.checkAfter;
        return ok({ RunId: CHECK_RUN_ID, RunNumber: 31 }, 202);
      }
      if (action === 'rollback') {
        return state.failRollback ? state.failRollback.clone() : ok({ RunId: ROLLBACK_RUN_ID, RunNumber: 32 }, 202);
      }
      return backendFailure('NOT_FOUND', 'x', 404);
    });
  };

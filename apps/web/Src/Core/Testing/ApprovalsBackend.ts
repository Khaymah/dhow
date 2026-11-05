import { backendFailure, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';
import { PROJECT_ID, RUN_ID, runJobFixture, type TFixture } from '@/Core/Testing/ProjectsBackend';

export const APPROVAL_ID = '0198a000-0000-7000-8000-000000000d01';
export const OTHER_APPROVAL_ID = '0198a000-0000-7000-8000-000000000d02';
export const ENVIRONMENT_ID = '0198a000-0000-7000-8000-000000000a01';
export const GOOD_CODE = '123456';

export type TApprovalFixture = Record<string, unknown> & { Id: string };

export interface IApprovalWrite {
  Path: string;
  Body: Record<string, unknown> | null;
}

export interface IApprovalsState {
  approvals: TApprovalFixture[];
  mine: TApprovalFixture[];
  writes: IApprovalWrite[];
  mineQueries: string[];
  failWrite: Response | null;
  failList: boolean;
  hangList: boolean;
  failMine: boolean;
  hangMine: boolean;
  onDecided: () => void;
}

export const approvalFixture = (over: Record<string, unknown> = {}): TApprovalFixture => ({
  Id: APPROVAL_ID,
  ProjectId: PROJECT_ID,
  EnvironmentId: ENVIRONMENT_ID,
  Environment: 'production',
  RunId: RUN_ID,
  RunNumber: 7,
  Ref: 'refs/heads/main',
  Round: 1,
  RuleVersion: 3,
  State: 'pending',
  ExpiresAt: '2026-11-08T08:00:00Z',
  CreatedAt: '2026-11-05T06:00:00Z',
  ...over,
});

export const gatedJob = (gate: string, over: Partial<TFixture> = {}): TFixture =>
  runJobFixture({
    Id: '0198a000-0000-7000-8000-000000000802',
    Key: 'deploy',
    JobId: 'deploy',
    Name: 'deploy',
    State: 'waiting',
    Gate: gate,
    Environment: 'production',
    EnvironmentId: ENVIRONMENT_ID,
    CurrentJobId: undefined,
    StartedAt: undefined,
    FinishedAt: undefined,
    ...over,
  });

export const newApprovalsState = (): IApprovalsState => ({
  approvals: [approvalFixture()],
  mine: [approvalFixture()],
  writes: [],
  mineQueries: [],
  failWrite: null,
  failList: false,
  hangList: false,
  failMine: false,
  hangMine: false,
  onDecided: () => undefined,
});

const RUN_APPROVALS = /\/projects\/[^/]+\/runs\/[^/]+\/approvals$/;
const DECISION = /\/runs\/[^/]+\/approvals\/([0-9a-f-]{36})\/(approve|reject|bypass)$/;
const MINE = /\/organizations\/[^/]+\/deployment-approvals$/;
const STATES = { approve: 'approved', reject: 'rejected', bypass: 'bypassed' } as const;

const ok = (data: unknown): Response => envelope({ Success: true, Data: data });

const bodyOf = async (request: Request): Promise<Record<string, unknown> | null> => {
  try {
    return (await request.clone().json()) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const decide = (state: IApprovalsState, id: string, action: keyof typeof STATES, body: Record<string, unknown> | null): Response => {
  const found = state.approvals.find((item) => item.Id === id);
  if (found === undefined) return backendFailure('NOT_FOUND', 'x', 404);
  const next: TApprovalFixture = {
    ...found,
    State: STATES[action],
    DecidedAt: '2026-11-05T09:00:00Z',
    FactorKind: 'totp',
    ...(typeof body?.Comment === 'string' ? { Comment: body.Comment } : {}),
    ...(typeof body?.Reason === 'string' ? { BypassReason: body.Reason } : {}),
  };
  state.approvals = state.approvals.map((item) => (item.Id === id ? next : item));
  state.mine = state.mine.filter((item) => item.Id !== id);
  state.onDecided();
  return ok(next);
};

export const approvalsRoute =
  (state: IApprovalsState): TRouteHandler =>
  (request, url) => {
    const { pathname } = url;
    if (request.method === 'GET' && RUN_APPROVALS.test(pathname)) {
      if (state.hangList) return new Promise<Response>(() => undefined);
      return state.failList ? backendFailure('INTERNAL', 'تعذّر تحميل الطلبات.', 500) : ok(state.approvals);
    }
    if (request.method === 'GET' && MINE.test(pathname)) {
      state.mineQueries.push(url.search);
      if (state.hangMine) return new Promise<Response>(() => undefined);
      return state.failMine ? backendFailure('INTERNAL', 'تعذّر تحميل الطلبات.', 500) : ok(state.mine);
    }
    const match = DECISION.exec(pathname);
    if (request.method !== 'POST' || match === null) return undefined;
    const [, id = '', action = 'approve'] = match;
    return bodyOf(request).then((body) => {
      state.writes.push({ Path: pathname.replace(/^\/api\/v1/, ''), Body: body });
      if (state.failWrite) return state.failWrite.clone();
      return decide(state, id, action === 'reject' || action === 'bypass' ? action : 'approve', body);
    });
  };

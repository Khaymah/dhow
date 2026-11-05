import { backendFailure, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { secretFixture, variableFixture, type TSecretFixture, type TVariableFixture } from '@/Core/Testing/SecretsBackend';

export const ENVIRONMENT_NAME = 'production';
export const REVIEWER_ID = '0198a000-0000-7000-8000-0000000000b3';
export const TEAM_ID = '0198a000-0000-7000-8000-0000000000d1';
export const DEPLOY_RUN_ID = '0198a000-0000-7000-8000-000000000401';

export type TEnvironmentFixture = Record<string, unknown> & { Name: string };

export interface IEnvironmentWrite {
  Method: string;
  Path: string;
  Body: Record<string, unknown> | null;
}

export interface IEnvironmentsState {
  environments: TEnvironmentFixture[];
  deployments: Record<string, Record<string, unknown>[]>;
  secrets: Record<string, TSecretFixture[]>;
  variables: Record<string, TVariableFixture[]>;
  writes: IEnvironmentWrite[];
  failWrite: Response | null;
  failDeploymentsList: boolean;
}

export const rulesFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  AllowedRefs: [{ Kind: 'branch', Pattern: 'main' }],
  AllowPullRequests: false,
  Reviewers: { UserIds: [REVIEWER_ID], TeamIds: [], UserCount: 1, TeamCount: 0, IdsShown: true },
  RequiredApprovals: 1,
  AllowSelfApproval: false,
  WaitMinutes: 10,
  ApprovalTimeoutHours: 72,
  RequirePinnedImages: false,
  AllowedPlugins: [],
  RequiredIsolation: 'container',
  AllowAdminBypass: false,
  SupersedePending: true,
  ...over,
});

export const environmentFixture = (over: Record<string, unknown> = {}): TEnvironmentFixture => ({
  Id: '0198a000-0000-7000-8000-000000000a01',
  OrganizationId: '0198a000-0000-7000-8000-0000000000a1',
  ProjectId: PROJECT_ID,
  Name: ENVIRONMENT_NAME,
  Description: 'Live site',
  Version: 3,
  RuleVersion: 3,
  Rules: rulesFixture(),
  Paused: false,
  CreatedAt: '2026-11-01T08:00:00Z',
  UpdatedAt: '2026-11-02T08:00:00Z',
  ...over,
});

export const deploymentFixture = (number: number, over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: `0198a000-0000-7000-8000-00000000b${String(number).padStart(3, '0')}`,
  RunId: DEPLOY_RUN_ID,
  RunNumber: number,
  RunJobId: '0198a000-0000-7000-8000-000000000801',
  JobId: '0198a000-0000-7000-8000-000000000501',
  JobKey: 'deploy',
  Ref: 'refs/heads/main',
  Sha: '0123456789abcdef0123456789abcdef01234567',
  RuleVersion: 3,
  State: 'succeeded',
  CreatedAt: '2026-11-03T08:00:00Z',
  FinishedAt: '2026-11-03T08:02:00Z',
  ...over,
});

export const newEnvironmentsState = (): IEnvironmentsState => ({
  environments: [environmentFixture()],
  deployments: { [ENVIRONMENT_NAME]: [deploymentFixture(9)] },
  secrets: { [ENVIRONMENT_NAME]: [] },
  variables: { [ENVIRONMENT_NAME]: [] },
  writes: [],
  failWrite: null,
  failDeploymentsList: false,
});

export const environmentSecret = (over: Record<string, unknown> = {}): TSecretFixture =>
  secretFixture({ Name: 'PROD_TOKEN', Scope: 'environment', Overrides: [], ...over });

export const environmentVariable = (over: Record<string, unknown> = {}): TVariableFixture =>
  variableFixture({ Name: 'PROD_HOST', Scope: 'environment', Value: 'prod.example.com', ...over });

const PATH =
  /^\/api\/v1\/organizations\/[^/]+\/projects\/[^/]+\/environments(?:\/([^/]+)(?:\/(remove|pause|resume|deployments|secrets|variables)(?:\/([^/]+)(?:\/(remove))?)?)?)?$/;
const TEAMS = /^\/api\/v1\/organizations\/[^/]+\/teams$/;

const ok = (data: unknown, status = 200): Response => envelope({ Success: true, Data: data }, status);

const bodyOf = async (request: Request): Promise<Record<string, unknown> | null> => {
  try {
    return (await request.clone().json()) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const merge = (current: TEnvironmentFixture, body: Record<string, unknown>): TEnvironmentFixture => {
  const rules = (body.Rules ?? {}) as Record<string, unknown>;
  const reviewers = rules.Reviewers as { UserIds: string[]; TeamIds: string[] } | undefined;
  const nextRules = { ...(current.Rules as Record<string, unknown>), ...rules };
  if (reviewers !== undefined) {
    nextRules.Reviewers = {
      ...reviewers,
      UserCount: reviewers.UserIds.length,
      TeamCount: reviewers.TeamIds.length,
      IdsShown: true,
    };
  }
  return {
    ...current,
    Description: typeof body.Description === 'string' ? body.Description : current.Description,
    Rules: nextRules,
    Version: (current.Version as number) + 1,
    RuleVersion: (current.RuleVersion as number) + 1,
  };
};

const pageOf = (items: readonly unknown[], url: URL): Response => {
  const page = Number(url.searchParams.get('Page') ?? '1');
  const limit = Number(url.searchParams.get('Limit') ?? '25');
  return envelope({ Success: true, Data: items.slice((page - 1) * limit, page * limit), Meta: { Page: page, Limit: limit, Total: items.length } });
};

const replace = (state: IEnvironmentsState, next: TEnvironmentFixture): void => {
  state.environments = state.environments.map((item) => (item.Name === next.Name ? next : item));
};

const writeEnvironment = (state: IEnvironmentsState, request: Request, name: string | undefined, action: string | undefined, body: Record<string, unknown> | null): Response => {
  if (name === undefined) {
    const empty = environmentFixture({
      Name: body?.Name,
      Description: body?.Description ?? '',
      Version: 1,
      RuleVersion: 1,
      Rules: rulesFixture({ AllowedRefs: [], WaitMinutes: 0, Reviewers: { UserIds: [], TeamIds: [], UserCount: 0, TeamCount: 0, IdsShown: true } }),
    });
    const created = { ...merge(empty, { Rules: body?.Rules ?? {} }), Version: 1, RuleVersion: 1 };
    state.environments = [...state.environments, created];
    return ok(created, 201);
  }
  const current = state.environments.find((item) => item.Name === name);
  if (current === undefined) return backendFailure('ENVIRONMENT_NOT_FOUND', 'x', 404);
  if (action === 'remove') {
    state.environments = state.environments.filter((item) => item.Name !== name);
    return ok(current);
  }
  if (action === 'pause') {
    const next = { ...current, Paused: true, PausedReason: body?.Reason, PausedAt: '2026-11-04T08:00:00Z', Version: (current.Version as number) + 1 };
    replace(state, next);
    return ok(next);
  }
  if (action === 'resume') {
    const next = { ...current, Paused: false, PausedReason: undefined, PausedAt: undefined, Version: (current.Version as number) + 1 };
    replace(state, next);
    return ok(next);
  }
  if (request.method === 'PUT') {
    const next = merge(current, body ?? {});
    replace(state, next);
    return ok(next);
  }
  return backendFailure('NOT_FOUND', 'x', 404);
};

const secretsOf = (state: IEnvironmentsState, name: string, kind: string, key: string | undefined, remove: boolean, body: Record<string, unknown> | null): Response => {
  if (kind === 'secrets') {
    const list = state.secrets[name] ?? [];
    if (remove) {
      state.secrets[name] = list.filter((item) => item.Name !== key);
      return ok(list.find((item) => item.Name === key) ?? environmentSecret({ Name: key }));
    }
    const next = environmentSecret({ Name: key, Description: body?.Description ?? '', Version: 1, SizeBytes: typeof body?.Value === 'string' ? body.Value.length : 0 });
    state.secrets[name] = [...list.filter((item) => item.Name !== key), next];
    return ok(next);
  }
  const list = state.variables[name] ?? [];
  if (remove) {
    state.variables[name] = list.filter((item) => item.Name !== key);
    return ok(list.find((item) => item.Name === key) ?? environmentVariable({ Name: key }));
  }
  const next = environmentVariable({ Name: key, Value: body?.Value ?? '' });
  state.variables[name] = [...list.filter((item) => item.Name !== key), next];
  return ok(next);
};

export const environmentsRoute =
  (state: IEnvironmentsState): TRouteHandler =>
  (request, url) => {
    if (TEAMS.test(url.pathname) && request.method === 'GET') {
      return ok([{ Id: TEAM_ID, Name: 'Backend', Description: 'Backend team', MemberCount: 2, Bindings: [], CreatedAt: '2026-10-01T08:00:00Z' }]);
    }
    const match = PATH.exec(url.pathname);
    if (!match) return undefined;
    const [, name, action, key, keyAction] = match;
    if (request.method === 'GET') {
      if (name === undefined) return ok(state.environments);
      if (action === 'deployments') {
        return state.failDeploymentsList ? backendFailure('INTERNAL', 'تعذّر تحميل السجل.', 500) : pageOf(state.deployments[name] ?? [], url);
      }
      if (action === 'secrets') return ok(state.secrets[name] ?? []);
      if (action === 'variables') return ok(state.variables[name] ?? []);
      const found = state.environments.find((item) => item.Name === name);
      return found === undefined ? backendFailure('ENVIRONMENT_NOT_FOUND', 'x', 404) : ok(found);
    }
    return bodyOf(request).then((body) => {
      state.writes.push({ Method: request.method, Path: url.pathname.replace(/^\/api\/v1/, ''), Body: body });
      if (state.failWrite) return state.failWrite.clone();
      if (action === 'secrets' || action === 'variables') {
        return secretsOf(state, name ?? '', action, key, keyAction === 'remove', body);
      }
      return writeEnvironment(state, request, name, action, body);
    });
  };

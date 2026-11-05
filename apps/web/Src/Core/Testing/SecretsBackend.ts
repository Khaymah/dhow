import type { QueryClient } from '@tanstack/react-query';

import { backendFailure, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';

export const SECRET_VALUE = 'S3cr3t-value-that-must-never-be-shown-8f2a';
export const REGISTRY_PASSWORD = 'registry-password-that-must-never-be-shown-77c1';
export const DEPLOY_PROJECT_ID = '0198a000-0000-7000-8000-000000000301';
export const SECOND_PROJECT_ID = '0198a000-0000-7000-8000-000000000302';
export const CREDENTIAL_ID = '0198a000-0000-7000-8000-000000000901';

export type TSecretFixture = Record<string, unknown> & { Name: string };
export type TVariableFixture = Record<string, unknown> & { Name: string };
export type TCredentialFixture = Record<string, unknown> & { Id: string };

export interface ISecretsWrite {
  Method: string;
  Path: string;
  Body: Record<string, unknown> | null;
}

export interface ISecretsState {
  secrets: Record<string, TSecretFixture[]>;
  variables: Record<string, TVariableFixture[]>;
  credentials: Record<string, TCredentialFixture[]>;
  writes: ISecretsWrite[];
  failWrites: Response | null;
  failLists: Response | null;
}

export const secretFixture = (over: Record<string, unknown> = {}): TSecretFixture => ({
  Name: 'DEPLOY_TOKEN',
  Scope: 'organization',
  Description: 'Deploy token for the API',
  Version: 2,
  SizeBytes: 40,
  AvailableToPullRequests: false,
  Visibility: 'all_projects',
  ProjectIds: [],
  Overrides: [],
  CreatedAt: '2026-10-20T08:00:00Z',
  UpdatedAt: '2026-11-01T08:00:00Z',
  UpdatedByUserId: '0198a000-0000-7000-8000-0000000000b1',
  ...over,
});

export const variableFixture = (over: Record<string, unknown> = {}): TVariableFixture => ({
  Name: 'DEPLOY_HOST',
  Scope: 'organization',
  Value: 'app.example.com',
  Overrides: [],
  CreatedAt: '2026-10-20T08:00:00Z',
  UpdatedAt: '2026-11-01T08:00:00Z',
  UpdatedByUserId: '0198a000-0000-7000-8000-0000000000b1',
  ...over,
});

export const credentialFixture = (over: Record<string, unknown> = {}): TCredentialFixture => ({
  Id: CREDENTIAL_ID,
  Scope: 'organization',
  Host: 'ghcr.io',
  Username: 'deploy-bot',
  CreatedAt: '2026-10-20T08:00:00Z',
  UpdatedAt: '2026-11-01T08:00:00Z',
  UpdatedByUserId: '0198a000-0000-7000-8000-0000000000b1',
  ...over,
});

export const newSecretsState = (): ISecretsState => ({
  secrets: { organization: [], project: [] },
  variables: { organization: [], project: [] },
  credentials: { organization: [], project: [] },
  writes: [],
  failWrites: null,
  failLists: null,
});

const PATH =
  /^\/api\/v1\/organizations\/[^/]+(?:\/projects\/([^/]+))?\/(secrets|variables|registry-credentials)(?:\/([^/]+))?(?:\/(remove))?$/;

const ok = (data: unknown, status = 200): Response => envelope({ Success: true, Data: data }, status);

const bodyOf = async (request: Request): Promise<Record<string, unknown> | null> => {
  try {
    return (await request.clone().json()) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const upsertSecret = (list: TSecretFixture[], scope: string, name: string, body: Record<string, unknown>): TSecretFixture => {
  const before = list.find((item) => item.Name === name);
  const value = typeof body.Value === 'string' ? body.Value : '';
  const next = secretFixture({
    ...before,
    Name: name,
    Scope: scope,
    Description: body.Description ?? '',
    AvailableToPullRequests: body.AvailableToPullRequests ?? false,
    Visibility: body.Visibility ?? 'all_projects',
    ProjectIds: body.ProjectIds ?? [],
    Version: typeof before?.Version === 'number' ? before.Version + 1 : 1,
    SizeBytes: value.length,
  });
  const index = list.findIndex((item) => item.Name === name);
  if (index === -1) list.push(next);
  else list[index] = next;
  return next;
};

const writeSecret = (
  state: ISecretsState,
  scope: string,
  name: string,
  remove: boolean,
  body: Record<string, unknown> | null,
): Response => {
  const list = state.secrets[scope] ?? [];
  if (remove) {
    state.secrets[scope] = list.filter((item) => item.Name !== name);
    return ok(list.find((item) => item.Name === name) ?? secretFixture({ Name: name }));
  }
  return ok(upsertSecret(list, scope, name, body ?? {}));
};

const writeVariable = (
  state: ISecretsState,
  scope: string,
  name: string,
  remove: boolean,
  body: Record<string, unknown> | null,
): Response => {
  const list = state.variables[scope] ?? [];
  if (remove) {
    state.variables[scope] = list.filter((item) => item.Name !== name);
    return ok(list.find((item) => item.Name === name) ?? variableFixture({ Name: name }));
  }
  const next = variableFixture({ Name: name, Scope: scope, Value: body?.Value ?? '' });
  const index = list.findIndex((item) => item.Name === name);
  if (index === -1) list.push(next);
  else list[index] = next;
  return ok(next);
};

const writeCredential = (
  state: ISecretsState,
  scope: string,
  id: string | undefined,
  remove: boolean,
  body: Record<string, unknown> | null,
): Response => {
  const list = state.credentials[scope] ?? [];
  if (remove) {
    state.credentials[scope] = list.filter((item) => item.Id !== id);
    return ok(list.find((item) => item.Id === id) ?? credentialFixture());
  }
  if (id === undefined) {
    const created = credentialFixture({
      Id: `0198a000-0000-7000-8000-00000000099${list.length}`,
      Scope: scope,
      Host: body?.Host,
      Username: body?.Username,
    });
    list.push(created);
    return ok(created, 201);
  }
  const index = list.findIndex((item) => item.Id === id);
  const next = { ...list[index], Username: body?.Username } as TCredentialFixture;
  if (index !== -1) list[index] = next;
  return ok(next);
};

export const secretsRoute =
  (state: ISecretsState): TRouteHandler =>
  (request, url) => {
    const match = PATH.exec(url.pathname);
    if (!match) return undefined;
    const [, projectId, kind, key, action] = match;
    const scope = projectId === undefined ? 'organization' : 'project';
    if (request.method === 'GET') {
      if (state.failLists) return state.failLists.clone();
      const source = kind === 'secrets' ? state.secrets : kind === 'variables' ? state.variables : state.credentials;
      return ok(source[scope] ?? []);
    }
    return bodyOf(request).then((body) => {
      state.writes.push({ Method: request.method, Path: url.pathname.replace(/^\/api\/v1/, ''), Body: body });
      if (state.failWrites) return state.failWrites.clone();
      const remove = action === 'remove';
      if (kind === 'secrets') return writeSecret(state, scope, key ?? '', remove, body);
      if (kind === 'variables') return writeVariable(state, scope, key ?? '', remove, body);
      return writeCredential(state, scope, key, remove, body);
    });
  };

export const secretsFailure = (code: string, message: string, status: number): Response => backendFailure(code, message, status);

const dumpStorage = (storage: Storage): string[] =>
  Array.from({ length: storage.length }, (_, index) => {
    const key = storage.key(index) ?? '';
    return `${key}=${storage.getItem(key) ?? ''}`;
  });

export const browserTraces = (client: QueryClient, routerHref = ''): string =>
  [
    document.documentElement.outerHTML,
    window.location.href,
    routerHref,
    JSON.stringify(window.history.state),
    ...dumpStorage(window.localStorage),
    ...dumpStorage(window.sessionStorage),
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((query) => [query.queryKey, query.state.data]),
    ),
    JSON.stringify(
      client
        .getMutationCache()
        .getAll()
        .map((mutation) => mutation.state),
    ),
  ].join('\n');

import type { QueryClient } from '@tanstack/react-query';
import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { approvalsRoute, gatedJob, newApprovalsState } from '@/Core/Testing/ApprovalsBackend';
import { auditFixture, startBackend } from '@/Core/Testing/Backend';
import { boardEnvironmentFixture, boardFixture, boardRoute, boardTargetFixture, boardUrl, newBoardState, runningFixture } from '@/Core/Testing/BoardsBackend';
import { FakeEventSource } from '@/Core/Testing/FakeEventSource';
import { ENVIRONMENT_NAME, environmentsRoute, newEnvironmentsState } from '@/Core/Testing/EnvironmentsBackend';
import { detailFixture, ok, PROJECT_ID, projectFixture, projectsRoute, RUN_ID, runFixture } from '@/Core/Testing/ProjectsBackend';
import { settingsFixture, settingsRoute } from '@/Core/Testing/SettingsBackend';
import { arText } from '@/Core/Testing/Text';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

configure({ asyncUtilTimeout: 5000 });

const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g;
const OVERVIEW = 'GET /api/v1/organizations/{id}/overview';
const SHELL = ['GET /api/v1/me', 'GET /api/v1/meta', 'GET /api/v1/me/preferences', 'GET /api/v1/health/ready', OVERVIEW];
const ORG = 'GET /api/v1/organizations/{id}';
const JOBS = 'GET /api/v1/organizations/{id}/jobs';
const RUNNERS = 'GET /api/v1/organizations/{id}/runners';
const AUDIT = 'GET /api/v1/organizations/{id}/audit-events';
const MEMBERS = 'GET /api/v1/organizations/{id}/members';

const endpointOf = (request: Request): string => `${request.method} ${new URL(request.url).pathname.replace(UUID, '{id}')}`;

const requestOf = (request: Request): string => `${endpointOf(request)}${new URL(request.url).search}`;

const countsOf = (calls: readonly Request[], key: (request: Request) => string = requestOf): Record<string, number> => {
  const counts: Record<string, number> = {};
  for (const call of calls) {
    const endpoint = key(call);
    counts[endpoint] = (counts[endpoint] ?? 0) + 1;
  }
  return counts;
};

const repeated = (calls: readonly Request[]): string[] =>
  Object.entries(countsOf(calls))
    .filter(([, count]) => count > 1)
    .map(([endpoint, count]) => `${endpoint} x${count}`);

const settle = async (client: QueryClient, calls: readonly Request[], expected: readonly string[]) => {
  for (let round = 0; round < 2; round += 1) {
    await waitFor(() => {
      expect(calls.map(endpointOf)).toEqual(expect.arrayContaining([...expected]));
      expect(client.isFetching()).toBe(0);
    });
    await new Promise((resolve) => setTimeout(resolve, 60));
  }
};

const mount = (url: string, role = 'owner') => {
  const backend = startBackend({ role, audit: auditFixture(3) });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user };
};

const ROUTES: readonly [string, string, readonly string[]][] = [
  ['/', Ar['home.title'], [JOBS, RUNNERS, AUDIT, MEMBERS]],
  ['/jobs', Ar['jobs.title'], ['GET /api/v1/organizations/{id}/jobs']],
  ['/approvals', Ar['approvals.page.title'], ['GET /api/v1/organizations/{id}/deployment-approvals']],
  ['/runners', Ar['runners.title'], ['GET /api/v1/organizations/{id}/runners']],
  ['/runners/tokens', Ar['runners.tokens.title'], ['GET /api/v1/organizations/{id}/runner-registration-tokens']],
  ['/org/members', Ar['shell.nav.members'], [ORG, 'GET /api/v1/organizations/{id}/members']],
  ['/org/teams', Ar['shell.nav.teams'], ['GET /api/v1/organizations/{id}/teams']],
  ['/org/members/0198a000-0000-7000-8000-0000000000b3', Ar['shell.nav.members'], [ORG, MEMBERS, 'GET /api/v1/me/mfa']],
  [
    '/org/teams/0198a000-0000-7000-8000-0000000000d1',
    Ar['shell.nav.teams'],
    ['GET /api/v1/organizations/{id}/teams', 'GET /api/v1/organizations/{id}/teams/{id}'],
  ],
  ['/org/invitations', Ar['shell.nav.invitations'], ['GET /api/v1/organizations/{id}/invitations']],
  ['/org/api-keys', Ar['shell.nav.apiKeys'], ['GET /api/v1/organizations/{id}/api-tokens']],
  [
    '/org/secrets',
    Ar['shell.nav.secrets'],
    [
      'GET /api/v1/organizations/{id}/secrets',
      'GET /api/v1/organizations/{id}/variables',
      'GET /api/v1/organizations/{id}/registry-credentials',
    ],
  ],
  ['/org/settings', Ar['shell.nav.orgSettings'], [ORG, 'GET /api/v1/me/mfa']],
  ['/audit', Ar['audit.title'], ['GET /api/v1/organizations/{id}/audit-events', 'GET /api/v1/organizations/{id}/members']],
  ['/system', Ar['status.title'], []],
  ['/account/profile', Ar['shell.nav.profile'], []],
  [
    '/account/security',
    Ar['shell.nav.security'],
    [
      'GET /api/v1/me/mfa',
      'GET /api/v1/me/passkeys',
      'GET /api/v1/oidc/providers',
      'GET /api/v1/me/identities',
      'GET /api/v1/sessions',
      'GET /api/v1/me/api-tokens',
    ],
  ],
  ['/account/preferences', Ar['shell.nav.preferences'], []],
  ['/organizations', Ar['organizations.title'], []],
  ['/notifications', Ar['notifications.title'], []],
];

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('API requests per page', () => {
  it.each(ROUTES)('opens %s cold with only what the page shows, each once', async (url, title, own) => {
    const view = mount(url);
    await screen.findByRole('heading', { level: 1, name: title });
    const expected = [...SHELL, ...own];
    await settle(view.client, view.calls, expected);

    expect(repeated(view.calls)).toEqual([]);
    expect([...new Set(view.calls.map(endpointOf))].sort()).toEqual([...expected].sort());
  });

  it.each([
    ['owner', [JOBS, RUNNERS, AUDIT, MEMBERS]],
    ['admin', [JOBS, RUNNERS, AUDIT, MEMBERS]],
    ['maintainer', [JOBS, RUNNERS]],
    ['developer', [JOBS, RUNNERS]],
    ['viewer', [JOBS]],
  ] as const)('opens / cold as a %s with only the cards the role may read', async (role, own) => {
    const view = mount('/', role);
    await screen.findByRole('heading', { level: 1, name: Ar['home.title'] });
    const expected = [...SHELL, ...own];
    await settle(view.client, view.calls, expected);

    expect(repeated(view.calls)).toEqual([]);
    expect([...new Set(view.calls.map(endpointOf))].sort()).toEqual([...expected].sort());
    expect(view.calls.map(requestOf).filter((call) => call.startsWith(JOBS) || call.startsWith(RUNNERS) || call.startsWith(AUDIT))).toEqual(
      expect.arrayContaining(own.filter((endpoint) => endpoint !== MEMBERS).map((endpoint) => `${endpoint}?Page=1&Limit=5`)),
    );
  });

  it('opens /system/settings cold as the server owner with one request per settings list', async () => {
    const backend = startBackend({ instanceOwner: true, routes: [settingsRoute({ settings: settingsFixture(), puts: [], tests: 0 })] });
    const client = createQueryClient({ retry: false });
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: ['/system/settings'] }))} queryClient={client} />);
    await screen.findByRole('heading', { level: 1, name: Ar['shell.nav.systemSettings'] });
    const expected = [...SHELL, 'GET /api/v1/instance/settings', 'GET /api/v1/instance/settings/oidc'];
    await settle(client, backend.calls, expected);

    expect(repeated(backend.calls)).toEqual([]);
    expect([...new Set(backend.calls.map(endpointOf))].sort()).toEqual([...expected].sort());
  });

  it('shares one overview request between the home and the sidebar counters', async () => {
    const view = mount('/');
    await screen.findByRole('heading', { level: 1, name: Ar['home.title'] });
    await settle(view.client, view.calls, SHELL);
    const nav = screen.getByRole('navigation', { name: Ar['shell.nav.label'] });
    for (const label of [Ar['shell.nav.members'], Ar['shell.nav.jobs'], Ar['shell.nav.home']]) {
      await view.user.click(within(nav).getByRole('link', { name: label }));
      await settle(view.client, view.calls, []);
    }
    expect(countsOf(view.calls, endpointOf)[OVERVIEW]).toBe(1);
  });

  it('moves between pages without asking for anything twice', async () => {
    const view = mount('/');
    await screen.findByRole('heading', { level: 1, name: Ar['home.title'] });
    await settle(view.client, view.calls, SHELL);
    const nav = screen.getByRole('navigation', { name: Ar['shell.nav.label'] });

    for (const [label, title] of [
      [Ar['shell.nav.members'], Ar['shell.nav.members']],
      [Ar['shell.nav.audit'], Ar['audit.title']],
      [Ar['shell.nav.orgSettings'], Ar['shell.nav.orgSettings']],
      [Ar['shell.nav.system'], Ar['status.title']],
      [Ar['shell.nav.members'], Ar['shell.nav.members']],
      [Ar['shell.nav.home'], Ar['home.title']],
    ] as const) {
      await view.user.click(within(nav).getByRole('link', { name: label }));
      await screen.findByRole('heading', { level: 1, name: title });
      await settle(view.client, view.calls, []);
    }

    expect(repeated(view.calls.filter((call) => call.method === 'GET'))).toEqual([]);
  });

  it('still asks for the sessions again right after one is revoked', async () => {
    const view = mount('/account/security');
    await screen.findByRole('heading', { level: 1, name: Ar['shell.nav.security'] });
    await settle(view.client, view.calls, ['GET /api/v1/sessions']);
    const list = await screen.findByRole('list', { name: Ar['account.sessions.title'] });
    const firefox = (await within(list).findByText(`${isolate('Firefox')} على ${isolate('Windows')}`)).closest('li');
    if (!firefox) throw new Error('no row for Firefox');

    await view.user.click(within(firefox).getByRole('button', { name: Ar['sessions.revoke.aria'].replace('{device}', isolate(`${isolate('Firefox')} على ${isolate('Windows')}`)) }));
    await view.user.click(within(firefox).getByRole('button', { name: Ar['sessions.revoke.confirm'] }));

    await waitFor(() => {
      expect(countsOf(view.calls, endpointOf)['GET /api/v1/sessions']).toBe(2);
    });
    expect(within(list).queryByText(`${isolate('Firefox')} على ${isolate('Windows')}`)).not.toBeInTheDocument();
  });
});

describe('API requests of the environment pages', () => {
  const PROJECT = 'GET /api/v1/organizations/{id}/projects/{id}';
  const ENVIRONMENTS = `${PROJECT}/environments`;
  const ENVIRONMENT = `${ENVIRONMENTS}/${ENVIRONMENT_NAME}`;

  const mountEnvironments = (url: string, role: string) => {
    const backend = startBackend({
      role,
      routes: [environmentsRoute(newEnvironmentsState()), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) })],
    });
    const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
    const client = createQueryClient({ retry: false });
    render(<App router={router} queryClient={client} />);
    return { ...backend, client };
  };

  it.each([
    [`/projects/${PROJECT_ID}/environments`, 'owner', [PROJECT, ENVIRONMENTS, `${ENVIRONMENT}/deployments`]],
    [
      `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`,
      'owner',
      [
        PROJECT,
        ENVIRONMENT,
        `${ENVIRONMENT}/deployments`,
        `${ENVIRONMENT}/targets`,
        `${ENVIRONMENT}/secrets`,
        `${ENVIRONMENT}/variables`,
        'GET /api/v1/me/mfa',
        MEMBERS,
        'GET /api/v1/organizations/{id}/teams',
      ],
    ],
    [
      `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`,
      'viewer',
      [PROJECT, ENVIRONMENT, `${ENVIRONMENT}/deployments`, `${ENVIRONMENT}/targets`],
    ],
  ] as const)('opens %s cold as a %s with only what the page shows, each once', async (url, role, own) => {
    const view = mountEnvironments(url, role);
    await screen.findByRole('heading', { level: 1, name: url.endsWith('environments') ? 'Web app' : ENVIRONMENT_NAME });
    const expected = [...SHELL, ...own];
    await settle(view.client, view.calls, expected);

    expect(repeated(view.calls)).toEqual([]);
    expect([...new Set(view.calls.map(endpointOf))].sort()).toEqual([...expected].sort());
  });
});

describe('API requests of the run page with a deploy waiting for approval', () => {
  const PROJECT = 'GET /api/v1/organizations/{id}/projects/{id}';
  const RUN = `${PROJECT}/runs/{id}`;
  const OWN = [
    PROJECT,
    RUN,
    `${RUN}/approvals`,
    `${RUN}/artifacts`,
    'GET /api/v1/organizations/{id}/deployment-approvals',
  ];

  const mountRun = (role: string) => {
    const backend = startBackend({
      role,
      routes: [
        approvalsRoute(newApprovalsState()),
        environmentsRoute(newEnvironmentsState()),
        projectsRoute({
          project: () => ok(projectFixture({ CallerRole: role })),
          run: () => ok(detailFixture(runFixture({ State: 'running', FinishedAt: undefined }), [gatedJob('approval')])),
        }),
      ],
    });
    const router = createAppRouter(createMemoryHistory({ initialEntries: [`/projects/${PROJECT_ID}/runs/${RUN_ID}`] }));
    const client = createQueryClient({ retry: false });
    render(<App router={router} queryClient={client} />);
    return { ...backend, client };
  };

  it.each([
    ['developer', OWN],
    ['owner', [...OWN, `${PROJECT}/environments/${ENVIRONMENT_NAME}`]],
    ['viewer', OWN.filter((endpoint) => !endpoint.endsWith('deployment-approvals') && !endpoint.endsWith('artifacts'))],
  ] as const)('opens the run cold as a %s with each request once', async (role, own) => {
    const view = mountRun(role);
    await screen.findByRole('heading', { level: 1, name: arText('runs.row.number', { number: 7 }) });
    const expected = [...SHELL, ...own];
    await settle(view.client, view.calls, expected);

    expect(repeated(view.calls)).toEqual([]);
    expect([...new Set(view.calls.map(endpointOf))].sort()).toEqual([...expected].sort());
  });
});

describe('API requests of the deploy tab', () => {
  const PROJECT = 'GET /api/v1/organizations/{id}/projects/{id}';
  const BOARD = `${PROJECT}/deploy-board`;
  const APPROVALS = 'GET /api/v1/organizations/{id}/deployment-approvals';

  const mountBoard = (role: string, state = newBoardState()) => {
    const backend = startBackend({
      role,
      routes: [boardRoute(state), approvalsRoute(newApprovalsState()), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) })],
    });
    const router = createAppRouter(createMemoryHistory({ initialEntries: [boardUrl()] }));
    const client = createQueryClient({ retry: false });
    render(<App router={router} queryClient={client} />);
    return { ...backend, client, state };
  };

  it.each(['owner', 'maintainer', 'viewer'] as const)('opens the deploy tab cold as a %s with each request once', async (role) => {
    const view = mountBoard(role);
    await screen.findByText(arText('boards.deployed'));
    const expected = [...SHELL, PROJECT, BOARD, APPROVALS];
    await settle(view.client, view.calls, expected);

    expect(repeated(view.calls)).toEqual([]);
    expect([...new Set(view.calls.map(endpointOf))].sort()).toEqual([...expected].sort());
  });

  it('asks for the board once for a burst of events, and for nothing else of the board', async () => {
    const view = mountBoard('owner');
    await screen.findByText(arText('boards.deployed'));
    await settle(view.client, view.calls, [BOARD]);
    const source = [...FakeEventSource.instances].reverse().find((candidate) => candidate.url.endsWith('/events'));
    if (source === undefined) throw new Error('the shell opened no organization stream');
    source.open();

    view.state.board = boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ Running: runningFixture() })])]);
    view.state.etag = '"v2"';
    for (let seq = 1; seq <= 20; seq += 1) {
      source.emit('change', { Seq: seq, Type: 'release.switched', Status: 'running', ProjectId: PROJECT_ID, At: '2026-11-07T10:00:00Z' }, `1-${String(seq)}`);
    }
    await screen.findByRole('progressbar');
    await settle(view.client, view.calls, [BOARD]);

    expect(view.calls.filter((call) => endpointOf(call) === BOARD)).toHaveLength(2);
    expect(view.calls.filter((call) => endpointOf(call) === APPROVALS)).toHaveLength(1);
    expect(view.calls.filter((call) => endpointOf(call) === PROJECT)).toHaveLength(1);
  });
});

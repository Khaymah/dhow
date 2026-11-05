import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend } from '@/Core/Testing/Backend';
import {
  deploymentFixture,
  ENVIRONMENT_NAME,
  environmentFixture,
  environmentsRoute,
  newEnvironmentsState,
  type IEnvironmentsState,
} from '@/Core/Testing/EnvironmentsBackend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { findPage } from '@/Core/Testing/Pages';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const PASSWORD = 'correct horse battery staple';
const LIST_URL = `/projects/${PROJECT_ID}/environments`;
const PAGE_URL = `${LIST_URL}/${ENVIRONMENT_NAME}`;
const API = `/organizations/${ORG_ID}/projects/${PROJECT_ID}/environments`;

const mount = (url: string, role = 'owner', state: IEnvironmentsState = newEnvironmentsState(), project: Record<string, unknown> = {}) => {
  const backend = startBackend({
    role,
    routes: [environmentsRoute(state), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role, ...project })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

const fillName = async (view: ReturnType<typeof mount>, dialog: HTMLElement, name: string) => {
  await view.user.type(await within(dialog).findByLabelText(Ar['environments.form.name']), name);
};

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the environments tab', () => {
  it('lists the environments with what is deployed now and the pause', async () => {
    const state = newEnvironmentsState();
    state.environments = [
      environmentFixture(),
      environmentFixture({ Name: 'staging', Description: '', Paused: true, PausedReason: 'Database migration failed' }),
    ];
    state.deployments[ENVIRONMENT_NAME] = [deploymentFixture(10, { State: 'failed' }), deploymentFixture(9)];
    state.deployments.staging = [];
    mount(LIST_URL, 'owner', state);
    const main = await findPage('Web app');

    const production = (await within(main).findByText('production')).closest('li');
    if (!production) throw new Error('no row for production');
    expect(await within(production).findByText(arText('environments.deployed.now', { number: 9 }), { exact: false })).toBeInTheDocument();
    expect(within(production).queryByText(Ar['environments.row.paused'])).not.toBeInTheDocument();

    const staging = within(main).getByText('staging').closest('li');
    if (!staging) throw new Error('no row for staging');
    expect(await within(staging).findByText(Ar['environments.deployed.none'])).toBeInTheDocument();
    expect(within(staging).getByText(Ar['environments.row.paused'])).toBeInTheDocument();
    expect(within(staging).getByText(arText('environments.row.pausedReason', { reason: 'Database migration failed' }))).toBeInTheDocument();
  });

  it('is a tab of the project for every role', async () => {
    const view = mount(`/projects/${PROJECT_ID}`, 'viewer');
    const tabs = await screen.findByRole('navigation', { name: Ar['projects.nav.label'] });
    await view.user.click(within(tabs).getByRole('link', { name: Ar['projects.nav.environments'] }));
    const main = await findPage('Web app');
    expect(view.router.state.location.pathname).toBe(LIST_URL);
    expect(await within(main).findByText('production')).toBeInTheDocument();
  });

  it.each([
    ['owner', true],
    ['admin', true],
    ['maintainer', false],
    ['developer', false],
    ['viewer', false],
  ] as const)('shows the create button to a %s: %s', async (role, shown) => {
    mount(LIST_URL, role);
    const main = await findPage('Web app');
    await within(main).findByText('production');
    expect(within(main).queryAllByRole('button', { name: Ar['environments.add'] }).length > 0).toBe(shown);
  });

  it('shows the empty state, with the create button for an admin only', async () => {
    const state = newEnvironmentsState();
    state.environments = [];
    mount(LIST_URL, 'admin', state);
    const main = await findPage('Web app');
    expect(await within(main).findByText(Ar['environments.empty'])).toBeInTheDocument();
    expect(within(main).getByRole('button', { name: Ar['environments.add'] })).toBeInTheDocument();
  });

  it('says the project is archived and offers no create button', async () => {
    mount(LIST_URL, 'owner', newEnvironmentsState(), { State: 'archived' });
    const main = await findPage('Web app');
    expect(await within(main).findByText(Ar['projects.archived.blocked'])).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['environments.add'] })).not.toBeInTheDocument();
  });
});

describe('creating an environment', () => {
  it('opens the slide from the tab and sends the name, the description and the second step', async () => {
    const view = mount(LIST_URL);
    const main = await findPage('Web app');
    await view.user.click((await within(main).findAllByRole('button', { name: Ar['environments.add'] }))[0] ?? document.body);
    const dialog = await screen.findByRole('dialog', { name: Ar['environments.new.title'] });
    expect(view.router.state.location.href).toBe(`${LIST_URL}?slide=environment-new`);

    await fillName(view, dialog, 'staging');
    await view.user.type(within(dialog).getByLabelText(Ar['environments.form.description']), 'Test site');
    await view.user.click(await within(dialog).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.form.submit'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: API,
      Body: { Name: 'staging', Description: 'Test site', Rules: {}, CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await findPage('staging')).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe(`${LIST_URL}/staging`);
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

});

describe('creating an environment with rules', () => {
  it('sends the refs and the rules it was given, and only those', async () => {
    const view = mount(`${LIST_URL}?slide=environment-new`);
    const dialog = await screen.findByRole('dialog', { name: Ar['environments.new.title'] });
    await fillName(view, dialog, 'qa');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.refs.add'] }));
    await view.user.type(within(dialog).getByLabelText(Ar['environments.refs.pattern']), 'release/**');
    await view.user.selectOptions(within(dialog).getByLabelText(Ar['environments.refs.kind']), 'tag');
    await view.user.clear(within(dialog).getByLabelText(Ar['environments.rule.wait']));
    await view.user.type(within(dialog).getByLabelText(Ar['environments.rule.wait']), '5');
    await view.user.click(within(dialog).getByRole('checkbox', { name: new RegExp(Ar['environments.rule.pinned']) }));
    await view.user.click(await within(dialog).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.form.submit'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toEqual({
      Name: 'qa',
      Description: '',
      Rules: { AllowedRefs: [{ Kind: 'tag', Pattern: 'release/**' }], WaitMinutes: 5, RequirePinnedImages: true },
      CurrentFactor: { Method: 'password', Password: PASSWORD },
    });
  });

  it('refuses a bad name or pattern before asking the server', async () => {
    const view = mount(`${LIST_URL}?slide=environment-new`);
    const dialog = await screen.findByRole('dialog', { name: Ar['environments.new.title'] });
    await fillName(view, dialog, 'Bad Name');
    await view.user.click(await within(dialog).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.form.submit'] }));
    expect(await within(dialog).findByText(Ar['environments.error.name'])).toBeInTheDocument();

    await view.user.clear(within(dialog).getByLabelText(Ar['environments.form.name']));
    await fillName(view, dialog, 'qa');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.refs.add'] }));
    await view.user.type(within(dialog).getByLabelText(Ar['environments.refs.pattern']), 'refs/heads/main');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.form.submit'] }));
    expect(await within(dialog).findByText(Ar['environments.error.pattern.invalid'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('says the name is taken when the server answers ENVIRONMENT_EXISTS', async () => {
    const state = newEnvironmentsState();
    state.failWrite = backendFailure('ENVIRONMENT_EXISTS', 'توجد بيئة بهذا الاسم.', 409);
    const view = mount(`${LIST_URL}?slide=environment-new`, 'owner', state);
    const dialog = await screen.findByRole('dialog', { name: Ar['environments.new.title'] });
    await fillName(view, dialog, ENVIRONMENT_NAME);
    await view.user.click(await within(dialog).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.form.submit'] }));
    expect(await within(dialog).findByText(Ar['environments.error.taken'])).toBeInTheDocument();
  });

  it('keeps the slide open with the server message when the second step is wrong', async () => {
    const state = newEnvironmentsState();
    state.failWrite = backendFailure('INVALID_CURRENT_FACTOR', 'تعذّر التحقق من خطوتك الثانية.', 403);
    const view = mount(`${LIST_URL}?slide=environment-new`, 'owner', state);
    const dialog = await screen.findByRole('dialog', { name: Ar['environments.new.title'] });
    await fillName(view, dialog, 'staging');
    await view.user.click(await within(dialog).findByLabelText(Ar['factor.method.password']));
    await view.user.paste('wrong');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['environments.form.submit'] }));
    expect(await within(dialog).findByText('تعذّر التحقق من خطوتك الثانية.')).toBeInTheDocument();
    expect(screen.getByRole('dialog', { name: Ar['environments.new.title'] })).toBeInTheDocument();
  });
});

describe('pausing and resuming', () => {
  it('lets a maintainer pause with a reason and no second step', async () => {
    const view = mount(PAGE_URL, 'maintainer');
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.pause'] });
    await view.user.type(within(region).getByLabelText(Ar['environments.pause.reason']), 'Database migration failed');
    await view.user.click(within(region).getByRole('button', { name: Ar['environments.pause.submit'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({ Method: 'POST', Path: `${API}/${ENVIRONMENT_NAME}/pause`, Body: { Reason: 'Database migration failed' } });
    expect(await within(region).findByText('Database migration failed', { exact: false })).toBeInTheDocument();
    expect(within(region).queryByRole('button', { name: Ar['environments.resume'] })).not.toBeInTheDocument();
    expect(within(region).getByText(Ar['environments.resume.forbidden'])).toBeInTheDocument();
  });

  it('asks for a reason before it pauses', async () => {
    const view = mount(PAGE_URL, 'maintainer');
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.pause'] });
    await view.user.click(within(region).getByRole('button', { name: Ar['environments.pause.submit'] }));
    expect(await within(region).findByText(Ar['environments.pause.error.reason'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('shows a developer no pause control', async () => {
    mount(PAGE_URL, 'developer');
    const main = await findPage(ENVIRONMENT_NAME);
    await within(main).findByRole('region', { name: Ar['environments.group.rules'] });
    expect(within(main).queryByRole('region', { name: Ar['environments.group.pause'] })).not.toBeInTheDocument();
  });

  it('lets an admin resume with the second step', async () => {
    const state = newEnvironmentsState();
    state.environments = [environmentFixture({ Paused: true, PausedReason: 'Stop', PausedAt: '2026-11-04T08:00:00Z' })];
    const view = mount(PAGE_URL, 'admin', state);
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.pause'] });
    expect(await within(region).findByText(Ar['environments.pause.active'])).toBeInTheDocument();
    await view.user.click(await within(region).findByRole('button', { name: Ar['environments.resume'] }));
    await view.user.type(await within(region).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(region).getByRole('button', { name: Ar['environments.resume.confirm'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/${ENVIRONMENT_NAME}/resume`,
      Body: { CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await within(region).findByRole('button', { name: Ar['environments.pause.submit'] })).toBeInTheDocument();
  });
});

describe('removing an environment', () => {
  it('asks for the second step, removes, and goes back to the list', async () => {
    const view = mount(PAGE_URL);
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.danger'] });
    await view.user.click(await within(region).findByRole('button', { name: Ar['environments.remove'] }));
    expect(within(region).getByText(arText('environments.remove.question', { name: ENVIRONMENT_NAME }))).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
    await view.user.type(await within(region).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(region).getByRole('button', { name: Ar['environments.remove.confirm'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/${ENVIRONMENT_NAME}/remove`,
      Body: { CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await findPage('Web app')).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe(LIST_URL);
    expect(await screen.findByText(Ar['environments.empty'])).toBeInTheDocument();
  });

  it('can be cancelled without a request', async () => {
    const view = mount(PAGE_URL);
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.danger'] });
    await view.user.click(await within(region).findByRole('button', { name: Ar['environments.remove'] }));
    await view.user.click(within(region).getByRole('button', { name: Ar['mfa.cancel'] }));
    expect(within(region).getByRole('button', { name: Ar['environments.remove'] })).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it.each(['maintainer', 'developer', 'viewer'])('shows a %s no danger zone', async (role) => {
    mount(PAGE_URL, role);
    const main = await findPage(ENVIRONMENT_NAME);
    await within(main).findByRole('region', { name: Ar['environments.group.rules'] });
    expect(within(main).queryByRole('region', { name: Ar['environments.group.danger'] })).not.toBeInTheDocument();
  });
});

describe('an environment that is not there', () => {
  it('says so when the server answers 404', async () => {
    mount(`${LIST_URL}/missing`);
    await findPage('missing');
    expect(await screen.findByText(Ar['environments.notFound'])).toBeInTheDocument();
  });

  it('shows the not-found page for a malformed name', async () => {
    mount(`${LIST_URL}/Bad_Name`);
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
  });
});

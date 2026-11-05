import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { findPage } from '@/Core/Testing/Pages';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import {
  credentialFixture,
  CREDENTIAL_ID,
  newSecretsState,
  secretFixture,
  secretsRoute,
  SECRET_VALUE,
  browserTraces,
  variableFixture,
} from '@/Core/Testing/SecretsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g;

const seeded = () => {
  const state = newSecretsState();
  state.secrets.organization = [secretFixture()];
  state.variables.organization = [variableFixture()];
  state.credentials.organization = [credentialFixture()];
  state.secrets.project = [secretFixture({ Name: 'PROJECT_KEY', Scope: 'project', Overrides: ['organization'] })];
  state.variables.project = [variableFixture({ Name: 'PROJECT_HOST', Scope: 'project' })];
  state.credentials.project = [credentialFixture({ Scope: 'project', Host: 'registry.example.com' })];
  return state;
};

const mount = (url: string, role = 'owner', state = seeded()) => {
  const backend = startBackend({
    role,
    routes: [secretsRoute(state), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

const ORG_URL = '/org/secrets';
const PROJECT_URL = `/projects/${PROJECT_ID}/secrets`;

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the organization secrets page', () => {
  it('lists secrets, variables and registry accounts, with every write button for an owner', async () => {
    mount(ORG_URL);
    const main = await findPage(Ar['shell.nav.secrets']);
    expect(await within(main).findByText('DEPLOY_TOKEN')).toBeInTheDocument();
    expect(await within(main).findByText('DEPLOY_HOST')).toBeInTheDocument();
    expect(await within(main).findByText('ghcr.io')).toBeInTheDocument();
    for (const name of [Ar['secrets.add'], Ar['variables.add'], Ar['registry.add']]) {
      expect(within(main).getAllByRole('button', { name }).length).toBeGreaterThan(0);
    }
  });

  it('is reached from the side menu of an admin', async () => {
    const view = mount('/org/settings', 'admin');
    const nav = await screen.findByRole('navigation', { name: Ar['shell.nav.label'] });
    await view.user.click(within(nav).getByRole('link', { name: Ar['shell.nav.secrets'] }));
    expect(await findPage(Ar['shell.nav.secrets'])).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe(ORG_URL);
  });

  it('shows the names to a developer and nothing to write with', async () => {
    mount(ORG_URL, 'developer');
    const main = await findPage(Ar['shell.nav.secrets']);
    expect(await within(main).findByText('DEPLOY_TOKEN')).toBeInTheDocument();
    expect(await within(main).findByText('ghcr.io')).toBeInTheDocument();
    expect(within(main).queryByRole('button')).not.toBeInTheDocument();
    expect(within(main).getByText(Ar['secrets.readOnly'])).toBeInTheDocument();
  });

  it('shows the names to a maintainer too, who writes project secrets but not the organization ones', async () => {
    mount(ORG_URL, 'maintainer');
    const main = await findPage(Ar['shell.nav.secrets']);
    expect(await within(main).findByText('DEPLOY_TOKEN')).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['secrets.add'] })).not.toBeInTheDocument();
  });

  it('is hidden from a viewer, in the menu and by address', async () => {
    mount(ORG_URL, 'viewer');
    await screen.findByRole('navigation', { name: Ar['shell.nav.label'] });
    expect(screen.queryByRole('link', { name: Ar['shell.nav.secrets'] })).not.toBeInTheDocument();
    expect(screen.queryByText('DEPLOY_TOKEN')).not.toBeInTheDocument();
  });
});

describe('the secret slides', () => {
  it('opens the add-secret slide from the page, saves, closes, and leaves the value nowhere', async () => {
    const view = mount(ORG_URL);
    const main = await findPage(Ar['shell.nav.secrets']);
    const [add] = await within(main).findAllByRole('button', { name: Ar['secrets.add'] });
    await view.user.click(add ?? document.body);
    const dialog = await screen.findByRole('dialog', { name: Ar['secrets.new.title'] });
    expect(view.router.state.location.href).toBe(`${ORG_URL}?slide=secret-new`);

    await view.user.type(await within(dialog).findByLabelText(Ar['secrets.form.name']), 'NEW_KEY');
    await view.user.click(within(dialog).getByLabelText(Ar['secrets.form.value']));
    await view.user.paste(SECRET_VALUE);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['secrets.form.submit'] }));

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(view.router.state.location.href).toBe(ORG_URL);
    expect(await within(main).findByText('NEW_KEY')).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(1);

    const everything = `${browserTraces(view.client, view.router.state.location.href)}\n${JSON.stringify(view.router.history.location)}`;
    expect(everything).not.toContain(SECRET_VALUE);
  });

  it('opens a change-value slide from a deep link and writes a new version', async () => {
    const view = mount(`${ORG_URL}?slide=secret-edit-DEPLOY_TOKEN`);
    const dialog = await screen.findByRole('dialog', { name: arText('secrets.edit.title', { name: 'DEPLOY_TOKEN' }) });
    const field = await within(dialog).findByLabelText(Ar['secrets.form.value.edit']);
    await view.user.click(field);
    await view.user.paste(SECRET_VALUE);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['secrets.form.submit.edit'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(view.state.secrets.organization?.[0]?.Version).toBe(3);
    expect(document.documentElement.outerHTML).not.toContain(SECRET_VALUE);
  });

  it('opens the variable edit slide from a deep link', async () => {
    mount(`${ORG_URL}?slide=variable-edit-DEPLOY_HOST`);
    const variable = await screen.findByRole('dialog', { name: arText('variables.edit.title', { name: 'DEPLOY_HOST' }) });
    expect(await within(variable).findByLabelText(Ar['variables.form.value'])).toHaveValue('app.example.com');
  });

  it('opens the registry edit slide from a deep link', async () => {
    mount(`${ORG_URL}?slide=registry-edit-${CREDENTIAL_ID}`);
    const dialog = await screen.findByRole('dialog', { name: Ar['registry.edit.title'] });
    expect(await within(dialog).findByLabelText(Ar['registry.form.username'])).toHaveValue('deploy-bot');
  });

  it('refuses a malformed edit slide name like any unknown slide', async () => {
    mount(`${ORG_URL}?slide=secret-edit-1bad`);
    await findPage(Ar['shell.nav.secrets']);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('shows the server message when the slide cannot save', async () => {
    const state = seeded();
    const view = mount(`${ORG_URL}?slide=secret-new`, 'owner', state);
    const dialog = await screen.findByRole('dialog', { name: Ar['secrets.new.title'] });
    await view.user.type(await within(dialog).findByLabelText(Ar['secrets.form.name']), 'BAD_ONE');
    await view.user.click(within(dialog).getByLabelText(Ar['secrets.form.value']));
    await view.user.paste('tiny');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['secrets.form.submit'] }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(Ar['secrets.form.error.short']);
  });
});

describe('the project secrets tab', () => {
  it('is a tab of the project for a maintainer, who writes project secrets and variables', async () => {
    const view = mount(`/projects/${PROJECT_ID}`, 'maintainer');
    const tabs = await screen.findByRole('navigation', { name: Ar['projects.nav.label'] });
    await view.user.click(within(tabs).getByRole('link', { name: Ar['projects.nav.secrets'] }));
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(PROJECT_URL);
    });
    expect(await screen.findByText('PROJECT_KEY')).toBeInTheDocument();
    const main = await findPage('Web app');
    expect(within(main).getByText('PROJECT_KEY')).toBeInTheDocument();
    expect(within(main).getByText(Ar['secrets.overrides.organization'])).toBeInTheDocument();
    expect(await within(main).findByText('PROJECT_HOST')).toBeInTheDocument();
    expect(within(main).getAllByRole('button', { name: Ar['secrets.add'] }).length).toBeGreaterThan(0);
    expect(within(main).getAllByRole('button', { name: Ar['variables.add'] }).length).toBeGreaterThan(0);
    expect(within(main).queryByText('DEPLOY_TOKEN')).not.toBeInTheDocument();
  });

  it('writes a project secret to the project endpoint from the slide', async () => {
    const view = mount(`${PROJECT_URL}?slide=secret-new`, 'maintainer');
    const dialog = await screen.findByRole('dialog', { name: Ar['secrets.new.title'] });
    await view.user.type(await within(dialog).findByLabelText(Ar['secrets.form.name']), 'NEW_KEY');
    await view.user.click(within(dialog).getByLabelText(Ar['secrets.form.value']));
    await view.user.paste(SECRET_VALUE);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['secrets.form.submit'] }));
    await waitFor(() => {
      expect(view.state.writes[0]?.Path).toBe(`/organizations/${ORG_ID}/projects/${PROJECT_ID}/secrets/NEW_KEY`);
    });
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

  it('shows a developer the names with no write button', async () => {
    mount(PROJECT_URL, 'developer');
    const main = await findPage('Web app');
    expect(await within(main).findByText('PROJECT_KEY')).toBeInTheDocument();
    expect(within(main).queryByRole('button')).not.toBeInTheDocument();
  });

  it('gives a viewer no tab and no secrets', async () => {
    mount(`/projects/${PROJECT_ID}`, 'viewer');
    const tabs = await screen.findByRole('navigation', { name: Ar['projects.nav.label'] });
    expect(within(tabs).queryByRole('link', { name: Ar['projects.nav.secrets'] })).not.toBeInTheDocument();
  });

  it('shows a viewer who opens the address a plain refusal', async () => {
    mount(PROJECT_URL, 'viewer');
    await findPage('Web app');
    expect(await screen.findByText(Ar['secrets.forbidden'])).toBeInTheDocument();
    expect(screen.queryByText('PROJECT_KEY')).not.toBeInTheDocument();
  });
});

describe('the registry accounts of a project', () => {
  it('sit in the project settings, writable by a maintainer', async () => {
    mount(`/projects/${PROJECT_ID}/settings`, 'maintainer');
    const main = await findPage('Web app');
    expect(await within(main).findByText('registry.example.com')).toBeInTheDocument();
    expect(within(main).getAllByRole('button', { name: Ar['registry.add'] }).length).toBeGreaterThan(0);
  });

  it('show a developer the hosts and no button', async () => {
    mount(`/projects/${PROJECT_ID}/settings`, 'developer');
    const main = await findPage('Web app');
    expect(await within(main).findByText('registry.example.com')).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['registry.add'] })).not.toBeInTheDocument();
  });

  it('are not asked for at all when the caller is a viewer', async () => {
    const view = mount(`/projects/${PROJECT_ID}/settings`, 'viewer');
    await findPage('Web app');
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(view.calls.some((call) => call.url.includes('registry-credentials'))).toBe(false);
  });
});

describe('requests of the secrets pages', () => {
  it.each([ORG_URL, PROJECT_URL, `/projects/${PROJECT_ID}/settings`])('opens %s cold and asks for each list once', async (url) => {
    const view = mount(url);
    await findPage(url === ORG_URL ? Ar['shell.nav.secrets'] : 'Web app');
    await waitFor(() => {
      expect(view.client.isFetching()).toBe(0);
    });
    await new Promise((resolve) => setTimeout(resolve, 60));
    const counts: Record<string, number> = {};
    for (const call of view.calls) {
      const key = `${call.method} ${new URL(call.url).pathname.replace(UUID, '{id}')}`;
      counts[key] = (counts[key] ?? 0) + 1;
    }
    expect(Object.entries(counts).filter(([, count]) => count > 1)).toEqual([]);
    expect(Object.keys(counts).some((key) => key.includes('/secrets') || key.includes('registry-credentials'))).toBe(true);
  });
});

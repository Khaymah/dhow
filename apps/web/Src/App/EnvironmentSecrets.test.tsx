import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend } from '@/Core/Testing/Backend';
import {
  deploymentFixture,
  ENVIRONMENT_NAME,
  environmentSecret,
  environmentVariable,
  environmentsRoute,
  newEnvironmentsState,
  type IEnvironmentsState,
} from '@/Core/Testing/EnvironmentsBackend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { findPage } from '@/Core/Testing/Pages';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { browserTraces, SECRET_VALUE } from '@/Core/Testing/SecretsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const PAGE_URL = `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`;
const API = `/organizations/${ORG_ID}/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`;

const seeded = (): IEnvironmentsState => {
  const state = newEnvironmentsState();
  state.secrets[ENVIRONMENT_NAME] = [environmentSecret()];
  state.variables[ENVIRONMENT_NAME] = [environmentVariable()];
  return state;
};

const mount = (url: string, role = 'owner', state: IEnvironmentsState = seeded()) => {
  const backend = startBackend({
    role,
    routes: [environmentsRoute(state), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the secrets and variables of an environment', () => {
  it('lists them with every write button for an owner', async () => {
    mount(PAGE_URL);
    const main = await findPage(ENVIRONMENT_NAME);
    expect(await within(main).findByText('PROD_TOKEN')).toBeInTheDocument();
    expect(await within(main).findByText('PROD_HOST')).toBeInTheDocument();
    expect(within(main).getByText('prod.example.com')).toBeInTheDocument();
    expect(within(main).getAllByRole('button', { name: Ar['secrets.add'] }).length).toBeGreaterThan(0);
    expect(within(main).getAllByRole('button', { name: Ar['variables.add'] }).length).toBeGreaterThan(0);
  });

  it('shows a maintainer the names and nothing to write with, as only an admin manages an environment', async () => {
    mount(PAGE_URL, 'maintainer');
    const main = await findPage(ENVIRONMENT_NAME);
    const secrets = await within(main).findByRole('region', { name: Ar['secrets.group.secrets'] });
    expect(await within(secrets).findByText('PROD_TOKEN')).toBeInTheDocument();
    expect(within(secrets).queryByRole('button')).not.toBeInTheDocument();
    const variables = within(main).getByRole('region', { name: Ar['secrets.group.variables'] });
    expect(await within(variables).findByText('PROD_HOST')).toBeInTheDocument();
    expect(within(variables).queryByRole('button')).not.toBeInTheDocument();
    expect(within(main).getByText(Ar['secrets.readOnly'])).toBeInTheDocument();
  });

  it('is not shown to a viewer, and is not asked for', async () => {
    const view = mount(PAGE_URL, 'viewer');
    const main = await findPage(ENVIRONMENT_NAME);
    await within(main).findByRole('region', { name: Ar['environments.group.rules'] });
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(within(main).queryByRole('region', { name: Ar['secrets.group.secrets'] })).not.toBeInTheDocument();
    expect(view.calls.some((call) => call.url.includes('/secrets') || call.url.includes('/variables'))).toBe(false);
  });

});

describe('writing the secrets and variables of an environment', () => {
  it('writes a secret to the environment endpoint without a value left anywhere', async () => {
    const view = mount(`${PAGE_URL}?slide=secret-new`);
    const dialog = await screen.findByRole('dialog', { name: Ar['secrets.new.title'] });
    expect(within(dialog).queryByText(Ar['secrets.form.pullRequests'])).not.toBeInTheDocument();
    expect(within(dialog).queryByText(Ar['secrets.form.projects'])).not.toBeInTheDocument();
    await view.user.type(await within(dialog).findByLabelText(Ar['secrets.form.name']), 'NEW_KEY');
    await view.user.type(within(dialog).getByLabelText(Ar['secrets.form.description']), 'Key for deploys');
    await view.user.click(within(dialog).getByLabelText(Ar['secrets.form.value']));
    await view.user.paste(SECRET_VALUE);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['secrets.form.submit'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'PUT',
      Path: `${API}/secrets/NEW_KEY`,
      Body: { Value: SECRET_VALUE, Description: 'Key for deploys' },
    });
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    const main = await findPage(ENVIRONMENT_NAME);
    expect(await within(main).findByText('NEW_KEY')).toBeInTheDocument();
    expect(view.router.state.location.href).toBe(PAGE_URL);
    const everything = `${browserTraces(view.client, view.router.state.location.href)}\n${JSON.stringify(view.router.history.location)}`;
    expect(everything).not.toContain(SECRET_VALUE);
  });

  it('writes a new version from the change slide and never shows the old value', async () => {
    const view = mount(`${PAGE_URL}?slide=secret-edit-PROD_TOKEN`);
    const dialog = await screen.findByRole('dialog', { name: arText('secrets.edit.title', { name: 'PROD_TOKEN' }) });
    const field = await within(dialog).findByLabelText(Ar['secrets.form.value.edit']);
    expect(field).toHaveValue('');
    await view.user.click(field);
    await view.user.paste(SECRET_VALUE);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['secrets.form.submit.edit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Path).toBe(`${API}/secrets/PROD_TOKEN`);
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(browserTraces(view.client, view.router.state.location.href)).not.toContain(SECRET_VALUE);
  });

  it('removes a secret from the change slide through the environment endpoint', async () => {
    const view = mount(`${PAGE_URL}?slide=secret-edit-PROD_TOKEN`);
    const dialog = await screen.findByRole('dialog', { name: arText('secrets.edit.title', { name: 'PROD_TOKEN' }) });
    await view.user.click(await within(dialog).findByRole('button', { name: Ar['secrets.remove'] }));
    await view.user.click(within(dialog).getByRole('button', { name: Ar['secrets.remove.confirm'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toMatchObject({ Method: 'POST', Path: `${API}/secrets/PROD_TOKEN/remove` });
  });

  it('adds, changes and removes a variable through the environment endpoint', async () => {
    const view = mount(`${PAGE_URL}?slide=variable-new`);
    const dialog = await screen.findByRole('dialog', { name: Ar['variables.new.title'] });
    await view.user.type(await within(dialog).findByLabelText(Ar['variables.form.name']), 'REGION');
    await view.user.type(within(dialog).getByLabelText(Ar['variables.form.value']), 'eu-west');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['variables.form.submit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({ Method: 'PUT', Path: `${API}/variables/REGION`, Body: { Value: 'eu-west' } });
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    const main = await findPage(ENVIRONMENT_NAME);
    const row = (await within(main).findByText('REGION')).closest('li');
    if (!row) throw new Error('no row for REGION');
    await view.user.click(within(row).getByRole('button', { name: arText('variables.remove.aria', { name: 'REGION' }) }));
    await view.user.click(await within(main).findByRole('button', { name: Ar['variables.remove.confirm'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(2);
    });
    expect(view.state.writes[1]).toMatchObject({ Method: 'POST', Path: `${API}/variables/REGION/remove` });
    await waitFor(() => {
      expect(within(main).queryByText('REGION')).not.toBeInTheDocument();
    });
  });
});

describe('the deployment log', () => {
  it('shows the first page and goes to the next', async () => {
    const state = seeded();
    state.deployments[ENVIRONMENT_NAME] = Array.from({ length: 25 }, (_, index) => deploymentFixture(25 - index, { State: index === 0 ? 'running' : 'succeeded' }));
    const view = mount(PAGE_URL, 'owner', state);
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.deployments'] });
    const list = await within(region).findByRole('list', { name: Ar['environments.deployments.list'] });
    expect(within(list).getAllByRole('listitem')).toHaveLength(20);
    expect(within(list).getByText(arText('environments.deployment.run', { number: 25 }))).toBeInTheDocument();
    expect(within(list).getByText(Ar['environments.deployment.state.running'])).toBeInTheDocument();
    expect(within(region).getByText(arText('environments.deployments.total', { count: 25 }))).toBeInTheDocument();

    await view.user.click(within(region).getByRole('button', { name: Ar['pager.next'] }));
    await waitFor(() => {
      expect(within(region).getByRole('list', { name: Ar['environments.deployments.list'] }).querySelectorAll('li')).toHaveLength(5);
    });
    expect(within(region).getByText(arText('environments.deployment.run', { number: 1 }))).toBeInTheDocument();
    expect(view.calls.some((call) => call.url.includes('/deployments') && call.url.includes('Page=2'))).toBe(true);
  });

  it('links a deployment to its run', async () => {
    mount(PAGE_URL);
    const main = await findPage(ENVIRONMENT_NAME);
    const region = await within(main).findByRole('region', { name: Ar['environments.group.deployments'] });
    const link = await within(region).findByRole('link', { name: /9/ });
    expect(link).toHaveAttribute('href', expect.stringMatching(/^\/projects\/[0-9a-f-]{36}\/runs\/[0-9a-f-]{36}$/));
  });

  it('says there is nothing yet', async () => {
    const state = seeded();
    state.deployments[ENVIRONMENT_NAME] = [];
    mount(PAGE_URL, 'owner', state);
    const main = await findPage(ENVIRONMENT_NAME);
    expect(await within(main).findByText(Ar['environments.deployments.empty'])).toBeInTheDocument();
  });

  it('says it could not load, and the rest of the page still works', async () => {
    const state = seeded();
    state.failDeploymentsList = true;
    mount(PAGE_URL, 'owner', state);
    const main = await findPage(ENVIRONMENT_NAME);
    expect(await within(main).findByText(Ar['environments.deployments.error'])).toBeInTheDocument();
    expect(within(main).getByLabelText(Ar['environments.rule.wait'])).toBeInTheDocument();
  });
});

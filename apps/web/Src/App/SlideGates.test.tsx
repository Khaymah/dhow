import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { slideAllowed, slideScopeOf } from '@/App/Panels';
import type { TRole } from '@/Core/Auth/AuthTypes';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend } from '@/Core/Testing/Backend';
import { ENVIRONMENT_NAME, environmentsRoute, newEnvironmentsState } from '@/Core/Testing/EnvironmentsBackend';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { newSecretsState, secretsRoute } from '@/Core/Testing/SecretsBackend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const ORG_URL = '/org/secrets';
const PROJECT_URL = `/projects/${PROJECT_ID}/secrets`;
const ENVIRONMENTS_URL = `/projects/${PROJECT_ID}/environments`;
const ENVIRONMENT_URL = `${ENVIRONMENTS_URL}/${ENVIRONMENT_NAME}`;

const mount = (url: string, role: string, projectRole: string = role) => {
  const backend = startBackend({
    role,
    routes: [
      secretsRoute(newSecretsState()),
      environmentsRoute(newEnvironmentsState()),
      projectsRoute({ project: () => ok(projectFixture({ CallerRole: projectRole })) }),
    ],
  });
  render(<App router={createAppRouter(createMemoryHistory({ initialEntries: [url] }))} queryClient={createQueryClient({ retry: false })} />);
  return backend;
};

const expectRefused = async () => {
  expect(await screen.findByText(Ar['toast.unknownPanel.title'])).toBeInTheDocument();
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
};

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the add slides of secrets, variables, registry accounts and environments', () => {
  it.each([
    ['developer', ORG_URL, 'secret-new'],
    ['developer', ORG_URL, 'variable-new'],
    ['developer', ORG_URL, 'registry-new'],
    ['maintainer', ORG_URL, 'secret-new'],
    ['maintainer', ORG_URL, 'variable-new'],
    ['maintainer', ORG_URL, 'registry-new'],
    ['viewer', ORG_URL, 'secret-new'],
    ['developer', PROJECT_URL, 'secret-new'],
    ['developer', PROJECT_URL, 'variable-new'],
    ['developer', PROJECT_URL, 'registry-new'],
    ['viewer', PROJECT_URL, 'secret-new'],
    ['maintainer', ENVIRONMENT_URL, 'secret-new'],
    ['maintainer', ENVIRONMENT_URL, 'variable-new'],
    ['developer', ENVIRONMENT_URL, 'secret-new'],
    ['maintainer', ENVIRONMENTS_URL, 'environment-new'],
    ['developer', ENVIRONMENTS_URL, 'environment-new'],
    ['viewer', ENVIRONMENTS_URL, 'environment-new'],
  ])('refuses %s at %s?slide=%s and keeps the page', async (role, url, slide) => {
    const { calls } = mount(`${url}?slide=${slide}`, role);
    await expectRefused();
    expect(calls.filter((call) => call.method !== 'GET')).toEqual([]);
  });

  it('judges a project slide by the role in the project, not by the role in the organization', async () => {
    mount(`${PROJECT_URL}?slide=secret-new`, 'maintainer', 'developer');
    await expectRefused();
  });

  it.each([
    ['admin', ORG_URL, 'secret-new', Ar['secrets.new.title']],
    ['admin', ORG_URL, 'variable-new', Ar['variables.new.title']],
    ['maintainer', PROJECT_URL, 'secret-new', Ar['secrets.new.title']],
    ['maintainer', PROJECT_URL, 'variable-new', Ar['variables.new.title']],
    ['admin', ENVIRONMENT_URL, 'secret-new', Ar['secrets.new.title']],
  ])('opens it for a %s at %s?slide=%s', async (role, url, slide, title) => {
    mount(`${url}?slide=${slide}`, role);
    expect(await screen.findByRole('dialog', { name: title })).toBeInTheDocument();
    expect(screen.queryByText(Ar['toast.unknownPanel.title'])).not.toBeInTheDocument();
  });

  it('opens a project slide for a member whose project role is higher than the organization role', async () => {
    mount(`${PROJECT_URL}?slide=secret-new`, 'developer', 'maintainer');
    expect(await screen.findByRole('dialog', { name: Ar['secrets.new.title'] })).toBeInTheDocument();
    expect(screen.queryByText(Ar['toast.unknownPanel.title'])).not.toBeInTheDocument();
  });

  it('opens the new environment slide for an admin', async () => {
    mount(`${ENVIRONMENTS_URL}?slide=environment-new`, 'admin');
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument();
    });
    expect(screen.queryByText(Ar['toast.unknownPanel.title'])).not.toBeInTheDocument();
  });
});

describe('the gate of a slide as a function', () => {
  const ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer', 'viewer'];

  it.each([
    ['secret-new', 'organization', ['owner', 'admin']],
    ['variable-new', 'organization', ['owner', 'admin']],
    ['registry-new', 'organization', ['owner', 'admin']],
    ['secret-new', 'project', ['owner', 'admin', 'maintainer']],
    ['variable-new', 'project', ['owner', 'admin', 'maintainer']],
    ['registry-new', 'project', ['owner', 'admin', 'maintainer']],
    ['secret-new', 'environment', ['owner', 'admin']],
    ['variable-new', 'environment', ['owner', 'admin']],
    ['environment-new', 'project', ['owner', 'admin']],
  ] as const)('%s in the %s scope is for %j only', (slide, scope, allowed) => {
    for (const role of ROLES) {
      expect(slideAllowed(slide, role, role, scope), `${slide} ${scope} ${role}`).toBe(allowed.some((name) => name === role));
    }
    expect(slideAllowed(slide, undefined, undefined, scope)).toBe(false);
  });

  it('reads the scope from the address', () => {
    expect(slideScopeOf(false, false)).toBe('organization');
    expect(slideScopeOf(true, false)).toBe('project');
    expect(slideScopeOf(true, true)).toBe('environment');
  });
});

import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope, ORG_ID } from '@/Core/Testing/Fetch';
import { GOOD_CODE } from '@/Core/Testing/MfaBackend';
import { findPage } from '@/Core/Testing/Pages';
import { jobFixture, jobsRoute, specFixture } from '@/Core/Testing/JobsBackend';
import { JOB_ID, ok, PROJECT_ID, projectFixture, projectsRoute, RUN_ID } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const organization = (days?: number): TRouteHandler => (_request, url) => {
  if (url.pathname !== `/api/v1/organizations/${ORG_ID}`) return undefined;
  return envelope({
    Success: true,
    Data: { Id: ORG_ID, Name: 'Future Solution', Slug: 'future-solution', CreatedAt: '2026-10-01T08:00:00Z', RequireSecondFactor: 'off', ...(days === undefined ? {} : { LogRetentionDays: days }) },
  });
};

const mount = (url: string, role = 'owner', routes: TRouteHandler[] = [], totp = false, project: Record<string, unknown> = {}) => {
  const backend = startBackend({
    role,
    routes: [...routes, projectsRoute({ project: () => ok(projectFixture({ CallerRole: role, ...project })) }), organization(30)],
    mfa: { TotpEnabled: totp, RecoveryCodesRemaining: totp ? 10 : 0, Passkeys: [] },
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  return { ...backend, router, user };
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the projects pages', () => {
  it('puts projects in the sidebar and the bottom bar and opens a project from the list', async () => {
    const view = mount('/projects', 'developer');
    const main = await findPage(Ar['projects.title']);
    const nav = screen.getByRole('navigation', { name: Ar['shell.bottom.label'] });
    expect(within(nav).getByRole('link', { name: Ar['shell.nav.projects'] })).toHaveAttribute('aria-current', 'page');
    expect(screen.getAllByRole('link', { name: Ar['shell.nav.projects'] }).length).toBeGreaterThan(1);
    expect(within(main).queryByRole('button', { name: Ar['projects.create'] })).toBeNull();
    await view.user.click(await within(main).findByRole('link', { name: /Web app/ }));
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(`/projects/${PROJECT_ID}`);
    });
    expect(await findPage('Web app')).toBeInTheDocument();
  });

  it('shows a project with its waiting repository, its runs and one primary action', async () => {
    const view = mount(`/projects/${PROJECT_ID}`, 'developer');
    const main = await findPage('Web app');
    expect(await within(main).findByText(Ar['repository.pending.title'])).toBeInTheDocument();
    expect(await within(main).findByRole('link', { name: new RegExp(arText('runs.row.number', { number: 7 })) })).toBeInTheDocument();
    await view.user.click(within(main).getByRole('button', { name: Ar['runs.trigger.open'] }));
    await waitFor(() => {
      expect(view.router.state.location.search).toEqual({ slide: 'run-trigger' });
    });
    expect(await screen.findByRole('heading', { name: Ar['runs.trigger.title'] })).toBeInTheDocument();
  });

  it('opens a run, links its jobs to the live log and names it in the crumbs', async () => {
    const view = mount(`/projects/${PROJECT_ID}/runs/${RUN_ID}`, 'maintainer');
    const main = await findPage(arText('runs.row.number', { number: 7 }));
    const job = await within(main).findByRole('link', { name: 'build' });
    expect(job).toHaveAttribute('href', `/jobs/${JOB_ID}`);
    expect(await screen.findByRole('navigation', { name: Ar['shell.crumbs.label'] })).toHaveTextContent(arText('runs.row.number', { number: 7 }));
    await view.user.click(within(main).getByRole('link', { name: arText('runs.detail.back', { name: 'Web app' }) }));
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(`/projects/${PROJECT_ID}`);
    });
  });

  it('shows the settings read-only to a developer and hides the danger zone', async () => {
    mount(`/projects/${PROJECT_ID}/settings`, 'developer');
    const main = await findPage('Web app');
    expect(await within(main).findByText(Ar['projects.settings.readOnly'])).toBeInTheDocument();
    expect(within(main).queryByText(Ar['projects.danger.title'])).toBeNull();
    expect(within(main).queryByRole('button', { name: Ar['repository.unlink.action'] })).toBeNull();
  });

  it('gives a maintainer the repository actions and the danger zone', async () => {
    mount(`/projects/${PROJECT_ID}/settings`, 'maintainer');
    const main = await findPage('Web app');
    expect(await within(main).findByRole('button', { name: Ar['repository.unlink.action'] })).toBeInTheDocument();
    expect(within(main).getByRole('button', { name: Ar['repository.actions.secret.action'] })).toBeInTheDocument();
  });

  it('refuses the new project slide to a developer by link', async () => {
    const view = mount('/projects?slide=project-new', 'developer');
    await findPage(Ar['projects.title']);
    await waitFor(() => {
      expect(view.router.state.location.search).toEqual({});
    });
  });

  it('shows a project that does not exist', async () => {
    mount(`/projects/${PROJECT_ID}`, 'owner', [
      (_request, url) => (url.pathname.endsWith(`/projects/${PROJECT_ID}`) ? envelope({ Success: false, Error: { Code: 'NOT_FOUND', Message: 'x' } }, 404) : undefined),
    ]);
    const main = await findPage(Ar['projects.detail.title']);
    expect(await within(main).findByText(Ar['projects.detail.notFound'])).toBeInTheDocument();
  });
});

describe('the manual run button', () => {
  it.each([
    ['viewer', {}, 'runs.trigger.forbidden'],
    ['developer', { RepositoryLinked: false }, 'runs.trigger.notLinked'],
    ['owner', { State: 'archived' }, 'runs.trigger.archived'],
  ] as const)('stays visible but disabled for a %s, with the reason', async (role, project, reason) => {
    const view = mount(`/projects/${PROJECT_ID}`, role, [], false, project);
    const main = await findPage('Web app');
    expect(await within(main).findByRole('button', { name: Ar['runs.trigger.open'] })).toBeDisabled();
    expect(within(main).getByText(Ar[reason])).toBeInTheDocument();
    if (reason === 'runs.trigger.notLinked') {
      await within(main).findByText(Ar['repository.empty.member']);
      expect(view.calls.some((call) => new URL(call.url).pathname.endsWith('/repository'))).toBe(false);
    }
  });
});

describe('a job of a run', () => {
  it('names the system job and links its run and project', async () => {
    const job = jobFixture({ Name: 'Read the pipeline file' });
    const view = mount(`/jobs/${job.Id}`, 'developer', [
      jobsRoute({
        jobs: [job],
        detail: () =>
          envelope({
            Success: true,
            Data: {
              Job: job,
              Spec: specFixture(),
              Attempts: [],
              Run: { ProjectId: PROJECT_ID, ProjectName: 'Web app', RunId: RUN_ID, RunNumber: 7, Kind: 'discovery' },
            },
          }),
      }),
    ]);
    const main = await findPage(Ar['jobs.detail.title']);
    expect(await within(main).findByRole('heading', { name: Ar['runs.job.system.discovery'] })).toBeInTheDocument();
    expect(within(main).queryByText('Read the pipeline file')).toBeNull();
    expect(within(main).getByRole('link', { name: arText('runs.row.number', { number: 7 }) })).toHaveAttribute('href', `/projects/${PROJECT_ID}/runs/${RUN_ID}`);
    expect(within(main).getByRole('link', { name: 'Web app' })).toHaveAttribute('href', `/projects/${PROJECT_ID}`);
    await view.user.click(within(main).getByRole('link', { name: arText('runs.row.number', { number: 7 }) }));
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(`/projects/${PROJECT_ID}/runs/${RUN_ID}`);
    });
    expect(await findPage(arText('runs.row.number', { number: 7 }))).toBeInTheDocument();
  });
});

describe('the log retention', () => {
  it('is read-only for an admin', async () => {
    mount('/org/settings', 'admin');
    const main = await findPage(Ar['shell.nav.orgSettings']);
    expect(await within(main).findByText(arText('logRetention.current.days', { days: 30 }))).toBeInTheDocument();
    expect(within(main).getByText(Ar['logRetention.ownerOnly'])).toBeInTheDocument();
    expect(within(main).queryByRole('spinbutton', { name: Ar['logRetention.field'] })).toBeNull();
  });

  it('asks an owner for at least seven days and a second factor', async () => {
    const view = mount('/org/settings', 'owner', [], true);
    const main = await findPage(Ar['shell.nav.orgSettings']);
    const field = await within(main).findByRole('spinbutton', { name: Ar['logRetention.field'] });
    await view.user.clear(field);
    await view.user.type(field, '3');
    await view.user.click(within(main).getByRole('button', { name: Ar['logRetention.review'] }));
    expect(await within(main).findByText(arText('logRetention.error.range', { min: 7, max: 3650 }))).toBeInTheDocument();
    await view.user.clear(field);
    await view.user.type(field, '14');
    await view.user.click(within(main).getByRole('button', { name: Ar['logRetention.review'] }));
    expect(await within(main).findByText(arText('logRetention.question.days', { days: 14 }))).toBeInTheDocument();
    await view.user.type(within(main).getByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await view.user.click(within(main).getByRole('button', { name: Ar['logRetention.confirm'] }));
    await waitFor(() => {
      expect(view.calls.some((call) => call.method === 'PUT' && call.url.endsWith('/log-retention'))).toBe(true);
    });
    const put = view.calls.find((call) => call.url.endsWith('/log-retention'));
    expect(await put?.clone().json()).toEqual({ LogRetentionDays: 14, CurrentFactor: { Method: 'totp', Code: GOOD_CODE } });
  });
});

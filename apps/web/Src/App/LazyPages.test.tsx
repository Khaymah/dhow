import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { approvalsRoute, newApprovalsState } from '@/Core/Testing/ApprovalsBackend';
import { auditFixture, startBackend, type TRouteHandler } from '@/Core/Testing/Backend';
import { ENVIRONMENT_NAME, environmentsRoute, newEnvironmentsState } from '@/Core/Testing/EnvironmentsBackend';
import { apiUp } from '@/Core/Testing/Fetch';
import { newTargetsManageState, targetsManageRoute } from '@/Core/Testing/TargetsManageBackend';
import { newTargetsState, targetsRoute } from '@/Core/Testing/TargetsBackend';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { Ar } from '@/I18n/Locales/Ar';

interface IPageCase {
  url: string;
  title: string;
  body: (main: HTMLElement) => Promise<HTMLElement>;
  routes?: () => TRouteHandler[];
}

const environmentRoutes = (): TRouteHandler[] => [
  environmentsRoute(newEnvironmentsState()),
  projectsRoute({ project: () => ok(projectFixture({ CallerRole: 'owner' })) }),
];

const targetRoutes = (): TRouteHandler[] => [
  targetsManageRoute(newTargetsManageState()),
  targetsRoute(newTargetsState()),
  ...environmentRoutes(),
];

const approvalRoutes = (): TRouteHandler[] => [approvalsRoute(newApprovalsState())];

configure({ asyncUtilTimeout: 5000 });

const CASES: IPageCase[] = [
  { url: '/system', title: Ar['status.title'], body: (main) => within(main).findByText('v0.1.0') },
  {
    url: '/system/checks/database',
    title: Ar['status.check.database'],
    body: (main) => within(main).findByText(Ar['checkDetail.database.means']),
  },
  { url: '/account/preferences', title: Ar['shell.nav.preferences'], body: (main) => within(main).findByText(Ar['appearance.system']) },
  { url: '/account/profile', title: Ar['shell.nav.profile'], body: (main) => within(main).findByText(Ar['account.profile.readOnly']) },
  { url: '/account/security', title: Ar['shell.nav.security'], body: (main) => within(main).findByText(Ar['account.sessions.title']) },
  { url: '/audit', title: Ar['audit.title'], body: (main) => within(main).findByRole('list', { name: Ar['audit.list.title'] }) },
  { url: '/org/members', title: Ar['shell.nav.members'], body: (main) => within(main).findByRole('list', { name: Ar['members.title'] }) },
  { url: '/notifications', title: Ar['notifications.title'], body: (main) => within(main).findByText(Ar['notifications.empty']) },
  {
    url: '/approvals',
    title: Ar['approvals.page.title'],
    body: (main) => within(main).findByRole('list', { name: Ar['approvals.page.list'] }),
    routes: approvalRoutes,
  },
  {
    url: `/projects/${PROJECT_ID}/environments`,
    title: 'Web app',
    body: (main) => within(main).findByRole('list', { name: Ar['environments.list'] }),
    routes: environmentRoutes,
  },
  {
    url: `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`,
    title: ENVIRONMENT_NAME,
    body: (main) => within(main).findByRole('region', { name: Ar['environments.group.rules'] }),
    routes: environmentRoutes,
  },
  {
    url: `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}/targets/web`,
    title: 'web',
    body: (main) => within(main).findByRole('list', { name: Ar['targets.hosts.title'] }),
    routes: targetRoutes,
  },
  {
    url: `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}/new-target`,
    title: Ar['targets.new.title'],
    body: (main) => within(main).findByRole('button', { name: Ar['targets.form.create'] }),
    routes: targetRoutes,
  },
];

const openFresh = async (url: string, title: string) => {
  vi.resetModules();
  const [{ App }, { createAppRouter }, { createQueryClient }] = await Promise.all([
    import('@/App/App'),
    import('@/App/Router'),
    import('@/Core/Api/QueryClient'),
  ]);
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const view = render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  await screen.findByRole('heading', { level: 1, name: title });
  return { router, view, main: screen.getByRole('main') };
};

describe('lazy pages opened from the address', () => {
  it.each(CASES)('opens $url on a cold load, and again after a reload', async ({ url, title, body, routes }) => {
    startBackend({ audit: auditFixture(3), routes: routes?.() });
    const first = await openFresh(url, title);
    expect(await body(first.main)).toBeInTheDocument();
    expect(first.router.state.location.href).toBe(url);
    first.view.unmount();

    const reloaded = await openFresh(url, title);
    expect(await body(reloaded.main)).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('ar');
  });
});

describe('a slide body that is still loading', () => {
  it('shows the skeleton in place of the body until its chunk arrives', async () => {
    vi.resetModules();
    apiUp();
    const { loadWorkspaceMessages } = await import('@/I18n/WorkspaceCatalog');
    await loadWorkspaceMessages('ar');
    const { renderWithProviders } = await import('@/Core/Testing/Render');
    const { LazyThemeEditorBody } = await import('@/App/Layout/LazySlides');
    renderWithProviders(<LazyThemeEditorBody onDone={() => undefined} />);
    const skeleton = screen.getByRole('status', { name: Ar['ui.loading'] });
    expect(skeleton).toHaveAttribute('aria-busy', 'true');
    expect(await screen.findByLabelText(Ar['themeEditor.name'])).toBeInTheDocument();
    expect(screen.queryByRole('status', { name: Ar['ui.loading'] })).not.toBeInTheDocument();
  });
});

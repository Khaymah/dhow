import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch } from '@/Core/Testing/Fetch';
import { ok, page, PROJECT_ID, projectFixture } from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { ProjectNewBody } from '@/Modules/Projects/Components/ProjectNewBody';
import { ProjectsBody, ProjectsFooter } from '@/Modules/Projects/Components/ProjectsBody';

const path = (request: Request) => new URL(request.url).pathname;

const mount = (role: string, list: (url: URL) => Response | Promise<Response>) => {
  const { calls } = stubFetch((request) => {
    if (path(request).endsWith('/me')) return envelope(meBody(role));
    if (path(request).endsWith('/projects')) return list(new URL(request.url));
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onOpenProject = vi.fn();
  const onCreate = vi.fn();
  const view = renderWithProviders(
    <>
      <ProjectsBody hrefFor={(id) => `/projects/${id}`} onOpenProject={onOpenProject} onCreate={onCreate} />
      <footer>
        <ProjectsFooter onCreate={onCreate} />
      </footer>
    </>,
  );
  return { ...view, calls, onOpenProject, onCreate };
};

describe('the projects list', () => {
  it('shows a skeleton while loading', async () => {
    mount('owner', () => new Promise<Response>(() => undefined));
    expect(await screen.findByRole('status', { name: Ar['projects.loading'] })).toBeInTheDocument();
  });

  it('shows the error with a retry that asks again', async () => {
    let fail = true;
    const view = mount('owner', () => (fail ? backendFailure('INTERNAL', 'تعطل السيرفر.', 500) : page([projectFixture()])));
    expect(await screen.findByText(Ar['projects.error'])).toBeInTheDocument();
    fail = false;
    await view.user.click(screen.getByRole('button', { name: Ar['projects.retry'] }));
    expect(await screen.findByRole('link', { name: /Web app/ })).toBeInTheDocument();
  });

  it('invites a maintainer to create the first project, and only tells a viewer who can', async () => {
    const view = mount('maintainer', () => page([]));
    expect(await screen.findByText(Ar['projects.empty.creator'])).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: Ar['projects.create'] })).toHaveLength(2);
    await view.user.click(within(screen.getByRole('contentinfo')).getByRole('button', { name: Ar['projects.create'] }));
    expect(view.onCreate).toHaveBeenCalledOnce();
    view.unmount();

    mount('viewer', () => page([]));
    expect(await screen.findByText(Ar['projects.empty.member'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['projects.create'] })).toBeNull();
  });

  it('lists projects as links and filters the archived ones on the server', async () => {
    const view = mount('developer', (url) =>
      page(url.searchParams.get('State') === 'archived' ? [projectFixture({ Name: 'Old app', State: 'archived' })] : [projectFixture()]),
    );
    const link = await screen.findByRole('link', { name: /Web app/ });
    expect(link).toHaveAttribute('href', `/projects/${PROJECT_ID}`);
    await view.user.click(link);
    expect(view.onOpenProject).toHaveBeenCalledWith(PROJECT_ID);
    await view.user.click(screen.getByRole('button', { name: Ar['projects.filter.archived'] }));
    expect(await screen.findByRole('link', { name: /Old app/ })).toBeInTheDocument();
    expect(screen.getByText(Ar['projects.state.archived'])).toBeInTheDocument();
    expect(view.calls.some((call) => new URL(call.url).searchParams.get('State') === 'archived')).toBe(true);
  });
});

const mountNew = (role: string, create: (body: Record<string, unknown>) => Response = (body) => ok(projectFixture(body), 201)) => {
  const bodies: Record<string, unknown>[] = [];
  stubFetch(async (request) => {
    if (path(request).endsWith('/me')) return envelope(meBody(role));
    if (request.method === 'POST' && path(request).endsWith(`/organizations/${ORG_ID}/projects`)) {
      const body = (await request.json()) as Record<string, unknown>;
      bodies.push(body);
      return create(body);
    }
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onCreated = vi.fn();
  return { ...renderWithProviders(<ProjectNewBody onCreated={onCreated} />), bodies, onCreated };
};

describe('the new project slide', () => {
  it('derives the slug, checks the fields and sends the project', async () => {
    const view = mountNew('admin');
    await view.user.click(await screen.findByRole('button', { name: Ar['projects.new.submit'] }));
    expect(await screen.findByText(Ar['projects.form.error.name'])).toBeInTheDocument();
    expect(view.bodies).toHaveLength(0);

    await view.user.type(screen.getByRole('textbox', { name: Ar['projects.form.name'] }), 'Web App');
    expect(screen.getByRole('textbox', { name: Ar['projects.form.slug'] })).toHaveValue('web-app');
    await view.user.click(screen.getByRole('radio', { name: new RegExp(Ar['projects.fork.automatic']) }));
    await view.user.click(screen.getByRole('button', { name: Ar['projects.new.submit'] }));
    await waitFor(() => {
      expect(view.onCreated).toHaveBeenCalledWith(PROJECT_ID);
    });
    expect(view.bodies[0]).toEqual({ Name: 'Web App', Slug: 'web-app', PipelinePath: '.dhow.yml', ForkPolicy: 'automatic' });
  });

  it('lets a maintainer create but not pick automatic fork runs, and shows the server error', async () => {
    const view = mountNew('maintainer', () => backendFailure('PROJECT_SLUG_TAKEN', 'المعرّف مستخدم.', 409));
    await view.user.type(await screen.findByRole('textbox', { name: Ar['projects.form.name'] }), 'Web');
    expect(screen.getByRole('radio', { name: new RegExp(Ar['projects.fork.automatic']) })).toBeDisabled();
    expect(screen.getByText(Ar['projects.fork.automatic.adminOnly'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['projects.new.submit'] }));
    expect(await screen.findByText('المعرّف مستخدم.')).toBeInTheDocument();
    expect(view.onCreated).not.toHaveBeenCalled();
  });

  it('refuses a developer', async () => {
    mountNew('developer');
    expect(await screen.findByText(Ar['projects.new.forbidden'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['projects.new.submit'] })).toBeNull();
  });

  it('explains the pipeline path with an example', async () => {
    mountNew('owner');
    expect(await screen.findByText(arText('projects.form.path.hint', { ext: '.yml / .yaml', example: '.dhow.yml' }))).toBeInTheDocument();
  });
});

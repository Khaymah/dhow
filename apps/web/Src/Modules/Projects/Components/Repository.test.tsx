import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch } from '@/Core/Testing/Fetch';
import { FINGERPRINT, notLinked, ok, PROJECT_ID, projectFixture, repositoryFixture, WEBHOOK_SECRET, type TFixture } from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { projectKey, repositoryKey, type TProject } from '@/Modules/Projects/Api/ProjectsApi';
import { ProjectDangerZone } from '@/Modules/Projects/Components/ProjectDangerZone';
import { ProjectSettingsCard } from '@/Modules/Projects/Components/ProjectSettingsCard';
import { RepositoryActions } from '@/Modules/Projects/Components/RepositoryActions';
import { RepositoryCard } from '@/Modules/Projects/Components/RepositoryCard';

const path = (request: Request) => new URL(request.url).pathname;

const backend = (repository: () => Response | Promise<Response>) => {
  const writes: { method: string; path: string; body: unknown }[] = [];
  let unlinked = false;
  stubFetch(async (request) => {
    const p = path(request);
    if (p.endsWith('/me')) return envelope(meBody('owner'));
    if (request.method !== 'GET') {
      const text = await request.text();
      writes.push({ method: request.method, path: p, body: text === '' ? null : JSON.parse(text) });
      if (request.method === 'DELETE') {
        unlinked = true;
        return ok(repositoryFixture());
      }
      if (p.endsWith('/rotate-webhook-secret')) return ok({ Repository: repositoryFixture(), WebhookSecret: WEBHOOK_SECRET });
      if (p.endsWith('/rotate-deploy-key')) return ok(repositoryFixture({ PreviousDeployKeyPublic: 'ssh-ed25519 OLD', PreviousDeployKeyExpiresAt: '2026-11-04T08:00:00Z' }));
      if (p.endsWith('/archive')) return ok(projectFixture({ State: 'archived' }));
      return ok(projectFixture(JSON.parse(text) as TFixture));
    }
    if (p.endsWith('/repository')) return unlinked ? notLinked() : repository();
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  return writes;
};

describe('the repository card', () => {
  it('covers loading, error and not linked for a manager and a member', async () => {
    backend(() => new Promise<Response>(() => undefined));
    const pending = renderWithProviders(<RepositoryCard organizationId={ORG_ID} projectId={PROJECT_ID} linked manager blocked={null} onLink={vi.fn()} />);
    expect(await screen.findByRole('status', { name: Ar['repository.loading'] })).toBeInTheDocument();
    pending.unmount();

    backend(() => backendFailure('INTERNAL', 'x', 500));
    const failing = renderWithProviders(<RepositoryCard organizationId={ORG_ID} projectId={PROJECT_ID} linked manager blocked={null} onLink={vi.fn()} />);
    expect(await screen.findByText(Ar['repository.error.load'])).toBeInTheDocument();
    failing.unmount();

    backend(notLinked);
    const onLink = vi.fn();
    const manager = renderWithProviders(<RepositoryCard organizationId={ORG_ID} projectId={PROJECT_ID} linked manager blocked={null} onLink={onLink} />);
    await manager.user.click(await screen.findByRole('button', { name: Ar['repository.link'] }));
    expect(onLink).toHaveBeenCalledOnce();
    manager.unmount();

    backend(notLinked);
    renderWithProviders(<RepositoryCard organizationId={ORG_ID} projectId={PROJECT_ID} linked manager={false} blocked={null} onLink={vi.fn()} />);
    expect(await screen.findByText(Ar['repository.empty.member'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['repository.link'] })).toBeNull();
  });

  it('shows the address, the webhook URL, the public key and a waiting fingerprint, never a secret', async () => {
    backend(() => ok(repositoryFixture()));
    renderWithProviders(<RepositoryCard organizationId={ORG_ID} projectId={PROJECT_ID} linked manager blocked={null} onLink={vi.fn()} />);
    expect(await screen.findByText('ssh://git@git.example.com/team/app.git')).toBeInTheDocument();
    expect(screen.getByText(Ar['repository.pending.title'])).toBeInTheDocument();
    expect(screen.getByRole('button', { name: Ar['repository.pending.confirm'] })).toBeInTheDocument();
    expect(screen.getByText(/webhooks\/git\//)).toBeInTheDocument();
    expect(screen.getByText(/ssh-ed25519/)).toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain(WEBHOOK_SECRET);
  });

  it('tells an archived project why nothing can change', async () => {
    backend(() => ok(repositoryFixture({ PendingHostKeyFingerprint: undefined, HostKeyFingerprint: FINGERPRINT, State: 'ready' })));
    renderWithProviders(<RepositoryCard organizationId={ORG_ID} projectId={PROJECT_ID} linked manager blocked={Ar['projects.archived.blocked']} onLink={vi.fn()} />);
    expect(await screen.findByText(Ar['projects.archived.blocked'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['repository.change'] })).toBeNull();
  });
});

describe('the repository keys and webhook', () => {
  it('shows a new webhook secret once after a confirmation, then hides it', async () => {
    const writes = backend(() => ok(repositoryFixture()));
    const view = renderWithProviders(<RepositoryActions organizationId={ORG_ID} projectId={PROJECT_ID} />);
    await view.user.click(await screen.findByRole('button', { name: Ar['repository.actions.secret.action'] }));
    expect(writes).toHaveLength(0);
    await view.user.click(screen.getByRole('button', { name: Ar['repository.actions.secret.confirm'] }));
    expect(await screen.findByText(WEBHOOK_SECRET)).toBeInTheDocument();
    expect(JSON.stringify(view.client.getQueryCache().getAll().map((query) => query.state.data))).not.toContain(WEBHOOK_SECRET);
    await view.user.click(screen.getByRole('button', { name: Ar['repository.secret.hide'] }));
    expect(screen.queryByText(WEBHOOK_SECRET)).toBeNull();
  });

  it('renews the deploy key and stops the previous one on request', async () => {
    const writes = backend(() => ok(repositoryFixture()));
    const view = renderWithProviders(<RepositoryActions organizationId={ORG_ID} projectId={PROJECT_ID} />);
    await view.user.click(await screen.findByRole('button', { name: Ar['repository.actions.key.action'] }));
    await waitFor(() => {
      expect(writes.map((write) => write.path.split('/').pop())).toEqual(['rotate-deploy-key']);
    });
  });

  it('unlinks only after a confirmation in the danger style', async () => {
    const writes = backend(() => ok(repositoryFixture()));
    const view = renderWithProviders(<ProjectDangerZone organizationId={ORG_ID} project={projectFixture() as TProject} />);
    view.client.setQueryData(projectKey(ORG_ID, PROJECT_ID), projectFixture());
    const unlink = await screen.findByRole('button', { name: Ar['repository.unlink.action'] });
    expect(unlink).toHaveAttribute('data-variant', 'danger');
    await view.user.click(unlink);
    expect(writes).toHaveLength(0);
    expect(screen.getByText(Ar['repository.unlink.question'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['repository.unlink.confirm'] }));
    await waitFor(() => {
      expect(writes.map((write) => write.method)).toEqual(['DELETE']);
    });
    await waitFor(() => {
      expect(view.client.getQueryData<TProject>(projectKey(ORG_ID, PROJECT_ID))?.RepositoryLinked).toBe(false);
    });
    expect(view.client.getQueryData(repositoryKey(ORG_ID, PROJECT_ID))).toBeUndefined();
  });

  it('archives after a confirmation', async () => {
    const writes = backend(() => ok(repositoryFixture()));
    const view = renderWithProviders(<ProjectDangerZone organizationId={ORG_ID} project={projectFixture() as TProject} />);
    await view.user.click(await screen.findByRole('button', { name: Ar['projects.archive.action'] }));
    await view.user.click(screen.getByRole('button', { name: Ar['projects.archive.confirm'] }));
    await waitFor(() => {
      expect(writes.map((write) => write.path.split('/').pop())).toEqual(['archive']);
    });
  });
});

describe('the project settings', () => {
  it('saves only what changed', async () => {
    const writes = backend(notLinked);
    const view = renderWithProviders(<ProjectSettingsCard organizationId={ORG_ID} project={projectFixture() as TProject} manager allowAutomatic />);
    const save = screen.getByRole('button', { name: Ar['projects.settings.save'] });
    expect(save).toBeDisabled();
    await view.user.click(screen.getByRole('radio', { name: new RegExp(Ar['projects.fork.automatic']) }));
    await view.user.click(save);
    await waitFor(() => {
      expect(writes[0]).toMatchObject({ method: 'PATCH', body: { ForkPolicy: 'automatic' } });
    });
  });

  it('is read-only for a developer and for an archived project, with the reason', () => {
    backend(notLinked);
    const developer = renderWithProviders(<ProjectSettingsCard organizationId={ORG_ID} project={projectFixture() as TProject} manager={false} allowAutomatic={false} />);
    expect(screen.getByText(Ar['projects.settings.readOnly'])).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: Ar['projects.form.name'] })).toBeDisabled();
    expect(screen.queryByRole('button', { name: Ar['projects.settings.save'] })).toBeNull();
    developer.unmount();
    renderWithProviders(<ProjectSettingsCard organizationId={ORG_ID} project={projectFixture({ State: 'archived' }) as TProject} manager allowAutomatic />);
    expect(screen.getByText(Ar['projects.settings.archived'])).toBeInTheDocument();
  });
});

import { act, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, stubFetch } from '@/Core/Testing/Fetch';
import {
  FINGERPRINT,
  notLinked,
  ok,
  PROJECT_ID,
  projectFixture,
  repositoryFixture,
  WEBHOOK_SECRET,
  type TFixture,
} from '@/Core/Testing/ProjectsBackend';
import { useSlideGuardStore } from '@/Core/Store/useSlideGuardStore';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { RepositoryLinkBody } from '@/Modules/Projects/Components/RepositoryLinkBody';

const path = (request: Request) => new URL(request.url).pathname;

interface IFlow {
  role?: string;
  repository?: TFixture | null;
  link?: (body: Record<string, unknown>) => Response;
}

const mount = ({ role = 'owner', repository = null, link }: IFlow = {}) => {
  let current: TFixture | null = repository;
  const writes: { method: string; path: string; body: unknown }[] = [];
  const { calls } = stubFetch(async (request) => {
    const p = path(request);
    if (p.endsWith('/me')) return envelope(meBody(role));
    if (request.method !== 'GET') {
      const body: unknown = request.headers.get('Content-Type')?.includes('json') ? await request.json() : null;
      writes.push({ method: request.method, path: p, body });
      if (p.endsWith('/repository') && request.method === 'PUT') {
        if (link) return link(body as Record<string, unknown>);
        const created = current === null;
        current = repositoryFixture({ CloneUrl: (body as { CloneUrl: string }).CloneUrl });
        return ok({ Repository: current, ...(created ? { WebhookSecret: WEBHOOK_SECRET } : {}) }, created ? 201 : 200);
      }
      if (p.endsWith('/rotate-webhook-secret')) return ok({ Repository: current, WebhookSecret: `${WEBHOOK_SECRET}-new` });
      if (p.endsWith('/confirm-host-key')) {
        current = repositoryFixture({ PendingHostKeyFingerprint: undefined, HostKeyFingerprint: FINGERPRINT, State: 'ready' });
        return ok(current);
      }
      return backendFailure('NOT_FOUND', 'x', 404);
    }
    if (p.endsWith(`/projects/${PROJECT_ID}`)) return ok(projectFixture({ CallerRole: role, RepositoryLinked: current !== null }));
    if (p.endsWith('/repository')) return current === null ? notLinked() : ok(current);
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onDone = vi.fn();
  const view = renderWithProviders(<RepositoryLinkBody projectId={PROJECT_ID} onDone={onDone} />);
  return { ...view, calls, writes, onDone };
};

afterEach(() => {
  useSlideGuardStore.setState({ Blocked: false, Attempts: 0 });
});

const storageText = (): string =>
  [window.localStorage, window.sessionStorage].map((store) => Object.keys(store).map((key) => `${key}=${store.getItem(key) ?? ''}`).join(';')).join('|');

describe('linking a generic Git repository', () => {
  it('walks the address, the read-only key, the pinned fingerprint and the secret shown once', async () => {
    const view = mount();
    expect(await screen.findByText(Ar['repository.source.generic'])).toBeInTheDocument();
    expect(screen.getByText(Ar['repository.source.github'])).toBeInTheDocument();
    expect(screen.getAllByText(Ar['repository.source.coming'])).toHaveLength(2);
    expect(screen.queryByRole('button', { name: /GitHub|GitLab/ })).toBeNull();

    await view.user.type(screen.getByRole('textbox', { name: Ar['repository.source.url'] }), 'file:///etc');
    await view.user.click(screen.getByRole('button', { name: Ar['repository.source.submit'] }));
    expect(await screen.findByText(Ar['repository.error.url'])).toBeInTheDocument();
    expect(view.writes).toHaveLength(0);

    await view.user.clear(screen.getByRole('textbox', { name: Ar['repository.source.url'] }));
    await view.user.type(screen.getByRole('textbox', { name: Ar['repository.source.url'] }), 'git@git.example.com:team/app.git');
    await view.user.click(screen.getByRole('button', { name: Ar['repository.source.submit'] }));

    expect(await screen.findByRole('code', { hidden: true, name: Ar['repository.deployKey.label'] })).toHaveTextContent('ssh-ed25519');
    expect(screen.getByText(Ar['repository.deployKey.readOnly.title'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['repository.deployKey.next'] }));

    expect(await screen.findByText(FINGERPRINT)).toBeInTheDocument();
    const confirm = screen.getByRole('button', { name: Ar['repository.hostKey.confirm'] });
    expect(confirm).toBeDisabled();
    expect(view.writes.some((write) => write.path.endsWith('/confirm-host-key'))).toBe(false);
    await view.user.click(screen.getByRole('checkbox', { name: Ar['repository.hostKey.checked'] }));
    await view.user.click(confirm);

    expect(await screen.findByText(WEBHOOK_SECRET)).toBeInTheDocument();
    expect(screen.getByText(Ar['repository.secret.once.title'])).toBeInTheDocument();
    expect(view.writes.find((write) => write.path.endsWith('/confirm-host-key'))?.body).toEqual({ Fingerprint: FINGERPRINT });
    expect(window.location.href).not.toContain(WEBHOOK_SECRET);
    expect(storageText()).not.toContain(WEBHOOK_SECRET);
    expect(view.client.getQueryCache().getAll().some((query) => JSON.stringify(query.state.data ?? null).includes(WEBHOOK_SECRET))).toBe(false);
    expect(view.client.getMutationCache().getAll().some((mutation) => JSON.stringify(mutation.state.data ?? null).includes(WEBHOOK_SECRET))).toBe(false);

    await view.user.click(screen.getByRole('button', { name: Ar['repository.webhook.next.copied'] }));
    expect(await screen.findByText(Ar['repository.status.waiting.title'])).toBeInTheDocument();
    expect(screen.queryByText(WEBHOOK_SECRET)).toBeNull();
    expect(document.body.innerHTML).not.toContain(WEBHOOK_SECRET);
    await view.user.click(screen.getByRole('button', { name: Ar['repository.status.done'] }));
    expect(view.onDone).toHaveBeenCalledOnce();
  });

  it('shows the translated server error for a refused address', async () => {
    const view = mount({ link: () => backendFailure('GIT_URL_INVALID', 'العنوان يشير إلى شبكة خاصة.', 422) });
    await view.user.type(await screen.findByRole('textbox', { name: Ar['repository.source.url'] }), 'git@10.0.0.1:a/b.git');
    await view.user.click(screen.getByRole('button', { name: Ar['repository.source.submit'] }));
    expect(await screen.findByText('العنوان يشير إلى شبكة خاصة.')).toBeInTheDocument();
    expect(screen.queryByText('GIT_URL_INVALID')).toBeNull();
  });

  it('opens at the fingerprint when one waits, and keeps the secret out when relinking', async () => {
    const view = mount({ repository: repositoryFixture() });
    expect(await screen.findByText(FINGERPRINT)).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['repository.hostKey.back'] }));
    expect(await screen.findByText(Ar['repository.source.relink.title'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['repository.source.submit.change'] }));
    await view.user.click(await screen.findByRole('button', { name: Ar['repository.deployKey.next'] }));
    await view.user.click(screen.getByRole('checkbox', { name: Ar['repository.hostKey.checked'] }));
    await view.user.click(screen.getByRole('button', { name: Ar['repository.hostKey.confirm'] }));
    expect(await screen.findByText(Ar['repository.webhook.kept.title'])).toBeInTheDocument();
    expect(screen.queryByText(WEBHOOK_SECRET)).toBeNull();
  });

  it('refuses a developer and an archived project', async () => {
    mount({ role: 'developer' });
    expect(await screen.findByText(Ar['repository.link.forbidden'])).toBeInTheDocument();
  });

  it('shows a skeleton while the project loads', async () => {
    stubFetch((request) => (path(request).endsWith('/me') ? envelope(meBody('owner')) : new Promise<Response>(() => undefined)));
    renderWithProviders(<RepositoryLinkBody projectId={PROJECT_ID} onDone={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByRole('status', { name: Ar['ui.loading'] })).toBeInTheDocument();
    });
  });
});

const toWebhookStep = async (view: ReturnType<typeof mount>) => {
  await view.user.type(await screen.findByRole('textbox', { name: Ar['repository.source.url'] }), 'git@git.example.com:team/app.git');
  await view.user.click(screen.getByRole('button', { name: Ar['repository.source.submit'] }));
  await view.user.click(await screen.findByRole('button', { name: Ar['repository.deployKey.next'] }));
  await view.user.click(screen.getByRole('checkbox', { name: Ar['repository.hostKey.checked'] }));
  await view.user.click(screen.getByRole('button', { name: Ar['repository.hostKey.confirm'] }));
  await screen.findByText(WEBHOOK_SECRET);
};

describe('keeping the webhook secret from being lost', () => {
  it('asks before the slide closes while the secret shows, and lets the user stay or leave', async () => {
    const view = mount();
    await toWebhookStep(view);
    expect(view.calls.filter((call) => call.method === 'GET' && call.url.endsWith('/repository'))).toHaveLength(0);
    expect(useSlideGuardStore.getState().Blocked).toBe(true);
    for (const button of screen.getAllByRole('button', { name: /انسخ/ })) expect(button).toHaveAttribute('data-variant', 'secondary');

    act(() => {
      useSlideGuardStore.getState().Attempt();
    });
    expect(await screen.findByText(Ar['repository.secret.leave.title'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['repository.secret.leave.stay'] }));
    expect(screen.queryByText(Ar['repository.secret.leave.title'])).toBeNull();
    expect(screen.getByText(WEBHOOK_SECRET)).toBeInTheDocument();

    act(() => {
      useSlideGuardStore.getState().Attempt();
    });
    await view.user.click(await screen.findByRole('button', { name: Ar['repository.secret.leave.confirm'] }));
    expect(view.onDone).toHaveBeenCalledOnce();
    expect(useSlideGuardStore.getState().Blocked).toBe(false);
  });

  it('lets the slide close freely once the user moved past the secret', async () => {
    const view = mount();
    await toWebhookStep(view);
    await view.user.click(screen.getByRole('button', { name: Ar['repository.webhook.next.copied'] }));
    expect(useSlideGuardStore.getState().Blocked).toBe(false);
  });

  it('resumes at the webhook step when reopened before the first webhook, and asks before a new secret', async () => {
    const view = mount({ repository: repositoryFixture({ PendingHostKeyFingerprint: undefined, HostKeyFingerprint: FINGERPRINT, State: 'ready' }) });
    expect(await screen.findByText(Ar['repository.webhook.kept.title'])).toBeInTheDocument();
    expect(screen.getByRole('listitem', { current: 'step' })).toHaveTextContent(Ar['repository.step.webhook']);
    await view.user.click(screen.getByRole('button', { name: Ar['repository.webhook.rotate'] }));
    expect(screen.getByText(Ar['repository.actions.secret.question'])).toBeInTheDocument();
    expect(view.writes).toHaveLength(0);
    await view.user.click(screen.getByRole('button', { name: Ar['repository.actions.secret.confirm'] }));
    expect(await screen.findByText(`${WEBHOOK_SECRET}-new`)).toBeInTheDocument();
    expect(useSlideGuardStore.getState().Blocked).toBe(true);
  });
});

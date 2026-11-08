import type { QueryClient } from '@tanstack/react-query';
import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { BACKOFF_MS, POLL_MS } from '@/Core/Events/OrgEventsConnection';
import { approvalFixture, approvalsRoute, newApprovalsState } from '@/Core/Testing/ApprovalsBackend';
import { startBackend } from '@/Core/Testing/Backend';
import {
  boardEnvironmentFixture,
  boardFixture,
  boardRoute,
  boardTargetFixture,
  boardUrl,
  newBoardState,
  runningFixture,
  type IBoardState,
} from '@/Core/Testing/BoardsBackend';
import { FakeEventSource } from '@/Core/Testing/FakeEventSource';
import { newTargetsState, targetsRoute } from '@/Core/Testing/TargetsBackend';
import { ok, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';

configure({ asyncUtilTimeout: 5000 });

const mount = (state: IBoardState, role = 'owner') => {
  const backend = startBackend({
    role,
    routes: [boardRoute(state), targetsRoute(newTargetsState()), approvalsRoute(newApprovalsState()), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [boardUrl()] }));
  const client: QueryClient = createQueryClient({ retry: false });
  render(<App router={router} queryClient={client} />);
  return { ...backend, client, router };
};

const stream = (): FakeEventSource => {
  const found = [...FakeEventSource.instances].reverse().find((source) => source.url.endsWith('/events'));
  if (found === undefined) throw new Error('the shell opened no organization stream');
  return found;
};

const boardHits = (calls: readonly Request[]): Request[] => calls.filter((call) => call.url.endsWith('/deploy-board'));

const event = (Type: string, over: Record<string, unknown> = {}) => ({
  Seq: 1,
  Type,
  Status: 'running',
  ProjectId: '0198a000-0000-7000-8000-000000000301',
  At: '2026-11-07T10:00:00Z',
  ...over,
});

afterEach(() => {
  vi.useRealTimers();
});

describe('the deploy tab', () => {
  it('opens as a tab of the project and shows the board with the actions of each target', async () => {
    mount(newBoardState());
    expect(await screen.findByRole('heading', { level: 1 })).toBeInTheDocument();
    expect(await screen.findByText(arText('boards.deployed'))).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: arText('projects.nav.label') });
    expect(within(nav).getByRole('link', { name: arText('projects.nav.deploy') })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('button', { name: arText('boards.action.check') })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: arText('boards.action.rollback') })).toBeInTheDocument();
  });

  it('gives a viewer the details but neither the check nor the rollback, and says why once', async () => {
    mount(newBoardState(), 'viewer');
    await screen.findByText(arText('boards.deployed'));
    expect(screen.queryByRole('button', { name: arText('boards.action.check') })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: arText('boards.action.details') })).toBeInTheDocument();
    expect(screen.getAllByText(arText('boards.action.forbidden'))).toHaveLength(1);
  });

  it('shows the deploys waiting for the reader, only those of this project', async () => {
    const approvals = newApprovalsState();
    approvals.mine = [approvalFixture(), approvalFixture({ Id: '0198a000-0000-7000-8000-000000000d09', ProjectId: '0198a000-0000-7000-8000-000000000399', Environment: 'other-project-env' })];
    const backend = startBackend({
      routes: [boardRoute(newBoardState()), approvalsRoute(approvals), projectsRoute({ project: () => ok(projectFixture({ CallerRole: 'owner' })) })],
    });
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: [boardUrl()] }))} queryClient={createQueryClient({ retry: false })} />);
    const card = (await screen.findByRole('list', { name: arText('boards.approvals.title') })).closest('section');
    if (card === null) throw new Error('the approvals are not in a card');
    expect(within(card).getAllByRole('listitem')).toHaveLength(1);
    expect(backend.calls.length).toBeGreaterThan(0);
  });
});

describe('the board while the stream is live', () => {
  it('refreshes by itself after a release event, without a reload, and only once for a burst', async () => {
    const state = newBoardState();
    const view = mount(state);
    await screen.findByText(arText('boards.deployed'));
    stream().open();
    expect(boardHits(view.calls)).toHaveLength(1);

    state.board = boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ Running: runningFixture() })])]);
    state.etag = '"v2"';
    for (let seq = 1; seq <= 6; seq += 1) stream().emit('change', event('release.switched', { Seq: seq }), `1-${String(seq)}`);

    expect(await screen.findByRole('progressbar')).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(boardHits(view.calls)).toHaveLength(2);
    expect(boardHits(view.calls)[1]?.headers.get('If-None-Match')).toBe('"v1"');
  });

  it('ignores an event of another project and refreshes the overview for every kind', async () => {
    const state = newBoardState();
    const view = mount(state);
    await screen.findByText(arText('boards.deployed'));
    stream().open();
    stream().emit('change', event('deployment.finished', { ProjectId: '0198a000-0000-7000-8000-0000000003ff' }), '1-1');
    await waitFor(() => {
      expect(view.calls.filter((call) => call.url.endsWith('/overview')).length).toBeGreaterThanOrEqual(2);
    });
    expect(boardHits(view.calls)).toHaveLength(1);
  });

  it('refetches everything after a reset', async () => {
    const state = newBoardState();
    const view = mount(state);
    await screen.findByText(arText('boards.deployed'));
    stream().open();
    stream().emit('reset', { Epoch: 9 });
    await waitFor(() => {
      expect(boardHits(view.calls)).toHaveLength(2);
    });
  });

  it('does not show the polling notice while the stream is live', async () => {
    mount(newBoardState());
    await screen.findByText(arText('boards.deployed'));
    stream().open();
    expect(screen.queryByText(arText('live.polling'))).not.toBeInTheDocument();
  });
});

describe('the board when the stream cannot work', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  it('falls back to a conditional request every 5 seconds, says so, and updates within 5 seconds', async () => {
    const state = newBoardState();
    const view = mount(state);
    await screen.findByText(arText('boards.deployed'));

    stream().fail(true);
    await vi.advanceTimersByTimeAsync(BACKOFF_MS[0]);
    stream().fail(true);
    await vi.advanceTimersByTimeAsync(BACKOFF_MS[1]);
    stream().fail(true);
    expect(await screen.findByText(arText('live.polling'))).toBeInTheDocument();
    expect(boardHits(view.calls)).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(POLL_MS + 400);
    await waitFor(() => {
      expect(boardHits(view.calls)).toHaveLength(2);
    });
    expect(boardHits(view.calls)[1]?.headers.get('If-None-Match')).toBe('"v1"');

    state.board = boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ CurrentReleaseId: 'rel-77', CurrentDigest: undefined })])]);
    state.etag = '"v2"';
    await vi.advanceTimersByTimeAsync(POLL_MS + 400);
    expect(await screen.findByText('rel-77')).toBeInTheDocument();
    expect(boardHits(view.calls)).toHaveLength(3);
  });

  it('goes back to the live stream when it opens again and drops the notice', async () => {
    const view = mount(newBoardState());
    await screen.findByText(arText('boards.deployed'));
    for (const delay of [BACKOFF_MS[0], BACKOFF_MS[1], 0]) {
      stream().fail(true);
      await vi.advanceTimersByTimeAsync(delay);
    }
    expect(await screen.findByText(arText('live.polling'))).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(60_000);
    stream().open();
    await waitFor(() => {
      expect(screen.queryByText(arText('live.polling'))).not.toBeInTheDocument();
    });
    expect(view.calls.length).toBeGreaterThan(0);
  });
});

describe('the actions of a target', () => {
  it('starts a check with one request and tells the user', async () => {
    const state = newBoardState();
    const view = mount(state);
    const user = userEvent.setup();
    await screen.findByText(arText('boards.deployed'));
    const checks: Request[] = [];
    const original = globalThis.fetch;
    vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : new Request(input, init);
      if (request.method === 'POST' && request.url.endsWith('/targets/web/check')) {
        checks.push(request);
        return Promise.resolve(new Response(JSON.stringify({ Success: true, Data: { RunId: '0198a000-0000-7000-8000-000000000f01', RunNumber: 12 } }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }
      return original(input, init);
    });
    await user.click(screen.getByRole('button', { name: arText('boards.action.check') }));
    await waitFor(() => {
      expect(checks).toHaveLength(1);
    });
    expect(await screen.findByText(arText('targets.check.started', { name: 'web' }))).toBeInTheDocument();
    expect(view.calls.length).toBeGreaterThan(0);
  });

  it('opens the releases of a target with the existing rollback button, and loads them only when asked', async () => {
    const view = mount(newBoardState());
    const user = userEvent.setup();
    await screen.findByText(arText('boards.deployed'));
    expect(view.calls.some((call) => call.url.includes('/targets'))).toBe(false);

    const toggle = screen.getByRole('button', { name: arText('boards.action.rollback') });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await user.click(toggle);
    expect(await screen.findByRole('heading', { name: arText('targets.releases.title') })).toBeInTheDocument();
    expect((await screen.findAllByRole('button', { name: arText('targets.rollback.action') })).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: arText('boards.action.hide') })).toHaveAttribute('aria-expanded', 'true');
    const targetCalls = view.calls.filter((call) => call.url.includes('/targets/web')).map((call) => new URL(call.url).pathname.split('/targets/web')[1]);
    expect([...new Set(targetCalls)].sort()).toEqual(['', '/check', '/releases']);

    await user.click(screen.getByRole('button', { name: arText('boards.action.hide') }));
    expect(screen.queryByRole('heading', { name: arText('targets.releases.title') })).not.toBeInTheDocument();
  });
});

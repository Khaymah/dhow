import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import {
  BOARD_DIGEST,
  boardEnvironmentFixture,
  boardFixture,
  boardRoute,
  boardTargetFixture,
  newBoardState,
  runningFixture,
  type IBoardState,
} from '@/Core/Testing/BoardsBackend';
import { notFound, stubFetch } from '@/Core/Testing/Fetch';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { DeployBoard } from '@/Modules/Boards/Components/DeployBoard';

const mount = (state: IBoardState = newBoardState()) => {
  const stub = stubFetch((request) => boardRoute(state)(request, new URL(request.url)) ?? notFound());
  const view = renderWithProviders(
    <DeployBoard
      organizationId={ORG_ID}
      projectId={PROJECT_ID}
      renderActions={(environment, target) => <button type="button">{`actions of ${environment.Name}/${target.Name}`}</button>}
    />,
  );
  return { ...view, ...stub, state };
};

const badgeOf = (word: HTMLElement): HTMLElement => {
  const badge = word.closest<HTMLElement>('[data-tone]');
  if (badge === null) throw new Error('the word is not in a badge');
  return badge;
};

const row = (target: string): HTMLElement => {
  const element = document.querySelector<HTMLElement>(`[data-target="${target}"]`);
  if (element === null) throw new Error(`no row for ${target}`);
  return element;
};

describe('DeployBoard', () => {
  it('shows a row for each environment with its target, the short digest and the status of the last deploy', async () => {
    mount();
    await screen.findByText(arText('boards.deployed'));
    const web = row('web');
    expect(within(web).getByText(BOARD_DIGEST.slice(0, 12))).toBeInTheDocument();
    expect(within(web).queryByText(BOARD_DIGEST)).not.toBeInTheDocument();
    expect(within(web).getByText(arText('boards.target.hosts', { count: 2 }))).toBeInTheDocument();
    const status = within(web).getByText(arText('boards.status.succeeded'));
    expect(badgeOf(status)).toHaveAttribute('data-tone', 'ok');
    expect(screen.getByRole('heading', { level: 4, name: 'production' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'actions of production/web' })).toBeInTheDocument();
  });

  it.each([
    ['running', 'info'],
    ['succeeded', 'ok'],
    ['failed', 'bad'],
    ['rolled_back', 'warn'],
    ['unknown', 'warn'],
  ] as const)('says %s with a word and an icon, not with a color alone', async (state, tone) => {
    mount(newBoardState(boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ Last: { ...(boardTargetFixture().Last as object), State: state } })])])));
    const word = await screen.findByText(arText(`boards.status.${state}`));
    const badge = badgeOf(word);
    expect(badge).toHaveAttribute('data-tone', tone);
    expect(badge.querySelector('svg')).not.toBeNull();
    expect(badge.textContent.trim()).toBe(arText(`boards.status.${state}`));
  });

});

describe('DeployBoard states', () => {
  it('says that a target rolled back by itself', async () => {
    mount(
      newBoardState(
        boardFixture([
          boardEnvironmentFixture({}, [
            boardTargetFixture({ AutoRolledBack: true, Last: { ...(boardTargetFixture().Last as object), State: 'rolled_back', RollbackReason: 'health_check_failed' } }),
          ]),
        ]),
      ),
    );
    const badge = badgeOf(await screen.findByText(arText('boards.status.rolled_back')));
    expect(badge).toHaveAttribute('data-tone', 'warn');
    expect(badge.querySelector('svg')).not.toBeNull();
  });

  it('keeps a red badge and a note on a target whose rollback failed, as long as the data says so', async () => {
    const state = newBoardState(
      boardFixture([
        boardEnvironmentFixture({}, [
          boardTargetFixture({ RollbackFailed: true, Partial: true, Last: { ...(boardTargetFixture().Last as object), State: 'rollback_failed' } }),
        ]),
      ]),
    );
    const view = mount(state);
    const badge = badgeOf(await screen.findByText(arText('boards.badge.rollbackFailed')));
    expect(badge).toHaveAttribute('data-tone', 'bad');
    expect(badge.querySelector('svg')).not.toBeNull();
    expect(screen.getByText(arText('boards.note.rollbackFailed'))).toBeInTheDocument();
    expect(badgeOf(screen.getByText(arText('boards.badge.partial')))).toHaveAttribute('data-tone', 'warn');

    await view.client.invalidateQueries();
    await waitFor(() => {
      expect(state.hits).toBe(2);
    });
    expect(screen.getByText(arText('boards.badge.rollbackFailed'))).toBeInTheDocument();
  });

  it('shows the batch progress of a running deploy as a progress bar with a word for each number', async () => {
    mount(newBoardState(boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ Running: runningFixture() })])])));
    const bar = await screen.findByRole('progressbar', { name: arText('boards.progress.label', { release: 'rel-42' }) });
    expect(bar).toHaveAttribute('aria-valuemin', '0');
    expect(bar).toHaveAttribute('aria-valuemax', '6');
    expect(bar).toHaveAttribute('aria-valuenow', '2');
    const text = `${arText('boards.progress.batch', { batch: 2, batches: 3 })} · ${arText('boards.progress.hosts', { done: 2, hosts: 6 })}`;
    expect(bar).toHaveAttribute('aria-valuetext', text);
    expect(within(bar).getByText(text)).toBeInTheDocument();
    expect([...bar.querySelectorAll('[data-segment]')].map((segment) => segment.getAttribute('data-segment'))).toEqual(['done', 'current', 'waiting']);
    expect(screen.getByText(arText('boards.status.running'))).toBeInTheDocument();
  });

  it('says a deploy is starting before any server switched', async () => {
    mount(newBoardState(boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ Running: runningFixture({ HostsDone: 0, Batch: 0 }) })])])));
    const bar = await screen.findByRole('progressbar');
    expect(within(bar).getByText(arText('boards.progress.starting', { hosts: 6 }))).toBeInTheDocument();
  });

  it('leaves out the segments for a deploy of very many batches and keeps the words', async () => {
    mount(newBoardState(boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ Running: runningFixture({ Hosts: 40, HostsDone: 10, BatchSize: 1, Batch: 11, Batches: 40 }) })])])));
    const bar = await screen.findByRole('progressbar');
    expect(bar.querySelectorAll('[data-segment]')).toHaveLength(0);
    expect(within(bar).getByText(new RegExp(arText('boards.progress.batch', { batch: 11, batches: 40 })))).toBeInTheDocument();
  });

  it('shows what an environment waits for and that it is paused', async () => {
    mount(newBoardState(boardFixture([boardEnvironmentFixture({ Paused: true, PendingApprovals: 2 })])));
    expect(badgeOf(await screen.findByText(arText('boards.env.paused')))).toHaveAttribute('data-tone', 'warn');
    expect(screen.getByText(arText('boards.env.approvals', { count: 2 }))).toBeInTheDocument();
  });

});

describe('DeployBoard edge cases', () => {
  it('shows an environment with no target and a target that was never deployed', async () => {
    mount(
      newBoardState(
        boardFixture([
          boardEnvironmentFixture({ Name: 'staging', Id: '0198a000-0000-7000-8000-000000000a02' }, []),
          boardEnvironmentFixture({ Name: 'qa', Id: '0198a000-0000-7000-8000-000000000a03' }, [
            boardTargetFixture({ Name: 'fresh', CurrentReleaseId: undefined, CurrentDigest: undefined, Last: undefined }),
          ]),
        ]),
      ),
    );
    expect(await screen.findByText(arText('boards.noTargets'))).toBeInTheDocument();
    const fresh = row('fresh');
    expect(within(fresh).getByText(arText('boards.deployed.none'))).toBeInTheDocument();
    expect(within(fresh).getByText(arText('boards.last.none'))).toBeInTheDocument();
  });

  it('falls back to the release id when the digest is unknown', async () => {
    mount(newBoardState(boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ CurrentDigest: undefined })])])));
    expect(await screen.findByText('rel-41')).toBeInTheDocument();
  });

  it('says when the list was cut at 500 targets', async () => {
    mount(newBoardState(boardFixture([boardEnvironmentFixture()], true)));
    expect(await screen.findByText(arText('boards.truncated'))).toBeInTheDocument();
  });

  it('says there are no environments and can retry after an error', async () => {
    const empty = mount(newBoardState(boardFixture([])));
    expect(await screen.findByText(arText('boards.empty'))).toBeInTheDocument();
    empty.unmount();

    const failing = newBoardState();
    failing.fail = true;
    const view = mount(failing);
    expect(await screen.findByText(arText('boards.error'))).toBeInTheDocument();
    failing.fail = false;
    await view.user.click(screen.getByRole('button', { name: arText('status.retry') }));
    expect(await screen.findByText(arText('boards.deployed'))).toBeInTheDocument();
  });
});

describe('DeployBoard requests', () => {
  it('sends the ETag back and keeps the same board when the server answers 304', async () => {
    const view = mount();
    await screen.findByText(arText('boards.deployed'));
    const before = document.querySelector('[data-target="web"]');
    expect(view.state.conditional).toEqual([null]);

    await view.client.invalidateQueries();
    await waitFor(() => {
      expect(view.state.hits).toBe(2);
    });
    expect(view.state.conditional).toEqual([null, '"v1"']);
    expect(document.querySelector('[data-target="web"]')).toBe(before);
  });

  it('takes the new board and the new ETag when something changed', async () => {
    const view = mount();
    await screen.findByText(arText('boards.deployed'));
    view.state.board = boardFixture([boardEnvironmentFixture({}, [boardTargetFixture({ CurrentReleaseId: 'rel-99', CurrentDigest: undefined })])]);
    view.state.etag = '"v2"';
    await view.client.invalidateQueries();
    expect(await screen.findByText('rel-99')).toBeInTheDocument();
    await view.client.invalidateQueries();
    await waitFor(() => {
      expect(view.state.conditional.at(-1)).toBe('"v2"');
    });
  });
});

describe('DeployBoard with a big project', () => {
  it('builds 200 environments with 2 targets each from one request and keeps the page small', async () => {
    const environments = Array.from({ length: 200 }, (_, index) =>
      boardEnvironmentFixture({ Id: `0198a000-0000-7000-8000-00000000${String(index).padStart(4, '0')}`, Name: `env-${String(index).padStart(3, '0')}` }, [
        boardTargetFixture({ Id: `0198a000-0000-7000-8000-10000000${String(index).padStart(4, '0')}`, Name: 'web', Running: index % 17 === 0 ? runningFixture() : undefined }),
        boardTargetFixture({ Id: `0198a000-0000-7000-8000-20000000${String(index).padStart(4, '0')}`, Name: 'api', RollbackFailed: index % 23 === 0, Partial: index % 23 === 0 }),
      ]),
    );
    const started = performance.now();
    const view = mount(newBoardState(boardFixture(environments)));
    await screen.findByRole('heading', { level: 4, name: 'env-199' });
    const elapsed = performance.now() - started;

    expect(view.state.hits).toBe(1);
    expect(document.querySelectorAll('[data-target]')).toHaveLength(400);
    expect(document.querySelectorAll('*').length).toBeLessThan(16_000);
    expect(document.querySelectorAll('[role="progressbar"]').length).toBeGreaterThan(0);
    expect(elapsed).toBeLessThan(15_000);
  });
});

import { screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, stubFetch } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { runnerFixture, runnerId, type TRunnerFixture } from '@/Core/Testing/RunnersBackend';
import { Ar } from '@/I18n/Locales/Ar';
import { WhyWaitingCard } from '@/Modules/Runners/Components/WhyWaitingCard';
import type { IJobNeeds } from '@/Modules/Runners/Types/RunnerMatch';
import { isolate } from '@/I18n/Translate';

const NEEDS: IJobNeeds = {
  Labels: ['linux-builders', 'arm64'],
  Isolation: 'container',
};

const page = (items: readonly TRunnerFixture[]) =>
  envelope({
    Success: true,
    Data: items,
    Meta: { Page: 1, Limit: 100, Total: items.length },
  });

const fill = (template: string, values: Record<string, string>) =>
  Object.entries(values).reduce((text, [key, value]) => text.replace(`{${key}}`, isolate(value)), template);

const mount = (runners: () => Response | Promise<Response>, role = 'owner', needs: IJobNeeds = NEEDS) => {
  const { calls } = stubFetch((request) => {
    const { pathname } = new URL(request.url);
    if (pathname.endsWith('/me')) return envelope(meBody(role));
    if (pathname.endsWith('/runners')) return runners();
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onAddRunner = vi.fn();
  const onOpenRunner = vi.fn();
  const view = renderWithProviders(<WhyWaitingCard needs={needs} onAddRunner={onAddRunner} onOpenRunner={onOpenRunner} />);
  const runnerCalls = () => calls.filter((call) => new URL(call.url).pathname.endsWith('/runners'));
  return { ...view, runnerCalls, onAddRunner, onOpenRunner };
};

const STUCK = [
  runnerFixture({
    Id: runnerId(1),
    Name: 'runner-01',
    Labels: ['linux-builders'],
  }),
  runnerFixture({
    Id: runnerId(2),
    Name: 'build-arm',
    Labels: ['linux-builders', 'arm64'],
    State: 'disabled',
    Status: 'disabled',
    DisabledReason: 'manual',
  }),
  runnerFixture({
    Id: runnerId(3),
    Name: 'gvisor-only',
    Labels: ['linux-builders', 'arm64'],
    IsolationLevels: ['gvisor'],
  }),
  runnerFixture({
    Id: runnerId(4),
    Name: 'old',
    Labels: ['linux-builders', 'arm64'],
    State: 'revoked',
    Status: 'revoked',
  }),
];

const rowOf = (table: HTMLElement, name: string): HTMLElement => {
  const row = within(table).getByText(name).closest('tr');
  if (row === null) throw new Error(`no row for ${name}`);
  return row;
};

describe('why a queued job is waiting', () => {
  it('shows a skeleton while the runners load', async () => {
    mount(() => new Promise<Response>(() => undefined));
    expect(await screen.findByRole('status', { name: Ar['runners.loading'] })).toBeInTheDocument();
  });

  it('shows the error and retries', async () => {
    let fail = true;
    const view = mount(() => (fail ? backendFailure('INTERNAL', 'تعطّل السيرفر.', 500) : page(STUCK)));
    expect(await screen.findByText(Ar['runners.error'])).toBeInTheDocument();
    fail = false;
    await view.user.click(screen.getByRole('button', { name: Ar['runners.retry'] }));
    expect(await screen.findByRole('table', { name: Ar['runners.fit.table'] })).toBeInTheDocument();
  });

  it('says there are no runners and offers to add one', async () => {
    const view = mount(() => page([runnerFixture({ State: 'revoked', Status: 'revoked' })]));
    expect(await screen.findByText(Ar['runners.fit.empty.manager'])).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runners.add'] }));
    expect(view.onAddRunner).toHaveBeenCalledOnce();
  });

  it('lists each active or paused runner with what it lacks', async () => {
    const view = mount(() => page(STUCK));
    const table = await screen.findByRole('table', {
      name: Ar['runners.fit.table'],
    });
    expect(within(table).getAllByRole('row')).toHaveLength(4);
    expect(within(table).queryByText('old')).not.toBeInTheDocument();

    const short = rowOf(table, 'runner-01');
    expect(within(short).getByText('arm64')).toBeInTheDocument();
    expect(within(short).queryByText('linux-builders')).not.toBeInTheDocument();
    expect(within(short).getByText(Ar['runners.fit.verdict.missingLabels'])).toBeInTheDocument();

    const paused = rowOf(table, 'build-arm');
    expect(within(paused).getByText(Ar['runners.fit.missing.none'])).toBeInTheDocument();
    expect(within(paused).getByText(Ar['runners.view.paused'])).toBeInTheDocument();
    expect(within(paused).getByText(Ar['runners.fit.verdict.paused'])).toBeInTheDocument();

    const gvisor = rowOf(table, 'gvisor-only');
    expect(within(gvisor).getByText(Ar['runners.fit.isolation.no'])).toBeInTheDocument();
    expect(within(gvisor).getByText(Ar['runners.fit.verdict.noIsolation'])).toBeInTheDocument();

    await view.user.click(
      screen.getByRole('button', {
        name: fill(Ar['runners.fit.resume'], { name: 'build-arm' }),
      }),
    );
    expect(view.onOpenRunner).toHaveBeenCalledWith(runnerId(2));
    expect(view.runnerCalls()).toHaveLength(1);
    expect(new URL(view.runnerCalls()[0]?.url ?? '').searchParams.get('Limit')).toBe('100');
  });

  it('says a matching runner can take it once it connects', async () => {
    mount(() =>
      page([
        runnerFixture({
          Name: 'arm-01',
          Labels: ['linux-builders', 'arm64'],
          Status: 'offline',
          LastSeenAt: undefined,
        }),
      ]),
    );
    const table = await screen.findByRole('table', {
      name: Ar['runners.fit.table'],
    });
    expect(within(rowOf(table, 'arm-01')).getByText(Ar['runners.fit.verdict.canTake'])).toBeInTheDocument();
  });

});

describe('the action under the table', () => {
  it('offers to edit the labels of the closest runner', async () => {
    const view = mount(() => page(STUCK.slice(0, 1)));
    await screen.findByRole('table', { name: Ar['runners.fit.table'] });
    await view.user.click(
      screen.getByRole('button', {
        name: fill(Ar['runners.fit.edit'], { name: 'runner-01' }),
      }),
    );
    expect(view.onOpenRunner).toHaveBeenCalledWith(runnerId(1));
  });

  it('offers to add a runner when no runner is close', async () => {
    const view = mount(() => page(STUCK.slice(2, 3)));
    await screen.findByRole('table', { name: Ar['runners.fit.table'] });
    await view.user.click(screen.getByRole('button', { name: Ar['runners.add'] }));
    expect(view.onAddRunner).toHaveBeenCalledOnce();
  });

});

describe('who sees the table', () => {
  it.each(['maintainer', 'developer'])('shows a %s the table and who can fix it, without actions', async (role) => {
    mount(() => page(STUCK), role);
    expect(await screen.findByRole('table', { name: Ar['runners.fit.table'] })).toBeInTheDocument();
    expect(screen.getByText(Ar['runners.fit.ask'])).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('shows a viewer nothing and asks for no runners', async () => {
    const view = mount(() => page(STUCK), 'viewer');
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    expect(screen.queryByText(Ar['runners.fit.title'])).not.toBeInTheDocument();
    expect(view.runnerCalls()).toEqual([]);
  });
});

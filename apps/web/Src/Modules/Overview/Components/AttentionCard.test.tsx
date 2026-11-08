import { screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { overviewFixture, startBackend, type IOverviewFixture } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { gate, type TGateMode } from '@/Core/Testing/Gate';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { AttentionCard } from '@/Modules/Overview/Components/AttentionCard';
import { useOverview } from '@/Modules/Overview/Hooks/useOverview';
import type { IOverviewLinks } from '@/Modules/Overview/Types/OverviewTargets';

const Harness = ({ failingChecks, links }: { failingChecks: number; links: IOverviewLinks }) => {
  const overview = useOverview(ORG_ID);
  return <AttentionCard overview={overview} failingChecks={failingChecks} links={links} />;
};

const mount = (overview: (role: string) => IOverviewFixture, options: { mode?: TGateMode; role?: string; failingChecks?: number } = {}) => {
  const overviewGate = gate(/\/overview$/, options.mode ?? 'pass');
  startBackend({ role: options.role, overview, routes: [overviewGate.route] });
  const links: IOverviewLinks = { href: (target) => `/${target}`, open: vi.fn() };
  const view = renderWithProviders(<Harness failingChecks={options.failingChecks ?? 0} links={links} />);
  return { ...view, overviewGate, links };
};

describe('AttentionCard', () => {
  it('shows a skeleton while the overview loads', async () => {
    mount(overviewFixture(), { mode: 'hang' });
    expect(await screen.findByRole('status', { name: Ar['home.attention.loading'] })).toBeInTheDocument();
  });

  it('shows an error and loads again on retry', async () => {
    const { overviewGate, user } = mount(overviewFixture({ Pending: 1 }), { mode: 'fail' });
    expect(await screen.findByText(Ar['home.overview.error'])).toBeInTheDocument();
    overviewGate.set('pass');
    await user.click(screen.getByRole('button', { name: Ar['jobs.retry'] }));
    expect(await screen.findByText(Ar['home.attention.invitations.title'])).toBeInTheDocument();
  });

  it('says in one calm line that all is well', async () => {
    mount(overviewFixture({ Jobs: { Queued: 3, Running: 1 }, Runners: { Online: 2 } }));
    expect(await screen.findByText(Ar['home.attention.calm'])).toBeInTheDocument();
    expect(screen.queryByRole('list')).not.toBeInTheDocument();
  });

  it('lists only what exists, with counts, and opens each', async () => {
    const { links, user } = mount(overviewFixture({ Jobs: { Queued: 4, QueuedNoRunner: 3 }, Runners: { Offline: 1 }, Pending: 2 }), {
      failingChecks: 1,
    });
    const list = await screen.findByRole('list', { name: Ar['home.attention.title'] });
    const rows = within(list).getAllByRole('listitem');
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining(Ar['home.attention.stuck.title']),
      expect.stringContaining(Ar['home.attention.checks.title']),
      expect.stringContaining(Ar['home.attention.offline.title']),
      expect.stringContaining(Ar['home.attention.invitations.title']),
    ]);
    expect(rows[0]).toHaveTextContent('3');
    await user.click(within(list).getByRole('button', { name: new RegExp(Ar['home.attention.offline.title']) }));
    expect(links.open).toHaveBeenCalledWith('offline');
  });

  it('leaves out the blocks the role may not read', async () => {
    mount(overviewFixture({ Jobs: { QueuedNoRunner: 1 }, Runners: { Offline: 5 }, Pending: 4 }), { role: 'viewer' });
    const list = await screen.findByRole('list', { name: Ar['home.attention.title'] });
    expect(within(list).getAllByRole('listitem')).toHaveLength(1);
    expect(within(list).queryByText(Ar['home.attention.offline.title'])).not.toBeInTheDocument();
    expect(within(list).queryByText(Ar['home.attention.invitations.title'])).not.toBeInTheDocument();
  });

  it('shows the deploys that need a person: a failed rollback first, then failed deploys and dead notifications for an owner', async () => {
    const { links, user } = mount(overviewFixture({ Deployments: { RollbackFailed: 2, FailedLast24h: 3 }, Dead: 4 }));
    const list = await screen.findByRole('list', { name: Ar['home.attention.title'] });
    expect(within(list).getAllByRole('listitem').map((row) => row.textContent)).toEqual([
      expect.stringContaining(Ar['home.attention.rollbackFailed.title']),
      expect.stringContaining(Ar['home.attention.failedDeploys.title']),
      expect.stringContaining(Ar['home.attention.deadNotifications.title']),
    ]);
    await user.click(within(list).getByRole('button', { name: new RegExp(Ar['home.attention.rollbackFailed.title']) }));
    expect(links.open).toHaveBeenCalledWith('rollbackFailed');
  });

  it('does not show dead notifications to a role that cannot manage them', async () => {
    mount(overviewFixture({ Deployments: { FailedLast24h: 1 }, Dead: 4 }), { role: 'maintainer' });
    const list = await screen.findByRole('list', { name: Ar['home.attention.title'] });
    expect(within(list).queryByText(Ar['home.attention.deadNotifications.title'])).not.toBeInTheDocument();
    expect(within(list).getByText(Ar['home.attention.failedDeploys.title'])).toBeInTheDocument();
  });
});

import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import type { TOverview } from '@/Modules/Overview/Api/OverviewApi';
import { OverviewStats } from '@/Modules/Overview/Components/OverviewStats';
import type { IOverviewLinks } from '@/Modules/Overview/Types/OverviewTargets';
import { isolate } from '@/I18n/Translate';

const FULL: TOverview = {
  Jobs: { Running: 1, Queued: 3, QueuedNoRunner: 2, FailedLast24h: 4 },
  Runners: { Online: 2, Offline: 1, Paused: 0 },
  Invitations: { Pending: 2 },
  Approvals: { Pending: 0 },
  Deployments: null,
  Notifications: null,
};

const mount = (overview: TOverview) => {
  const links: IOverviewLinks = { href: (target) => `/go/${target}`, open: vi.fn() };
  return { ...renderWithProviders(<OverviewStats overview={overview} links={links} />), links };
};

describe('OverviewStats', () => {
  it('shows the four numbers in the status colors, each a link to its filtered page', async () => {
    const { links, user } = mount(FULL);
    const tiles = screen.getAllByRole('link');
    expect(tiles.map((tile) => [tile.getAttribute('href'), tile.getAttribute('data-tone')])).toEqual([
      ['/go/running', 'info'],
      ['/go/queued', 'queue'],
      ['/go/failed', 'bad'],
      ['/go/runners', 'ok'],
    ]);
    expect(tiles.map((tile) => tile.querySelector('b')?.textContent)).toEqual(['1', '3', '4', '2']);
    expect(tiles[1]).toHaveTextContent(Ar['home.stat.queued.stuck'].replace('{count}', isolate('2')));
    expect(tiles[3]).toHaveTextContent(Ar['home.stat.online.offline'].replace('{count}', isolate('1')));
    await user.click(screen.getByRole('link', { name: new RegExp(Ar['home.stat.failed']) }));
    expect(links.open).toHaveBeenCalledWith('failed');
  });

  it('shows zeros as zeros, without notes', () => {
    mount({ Jobs: { Running: 0, Queued: 0, QueuedNoRunner: 0, FailedLast24h: 0 }, Runners: { Online: 0, Offline: 0, Paused: 0 }, Invitations: null, Approvals: null, Deployments: null, Notifications: null });
    expect(screen.getAllByRole('link').map((tile) => tile.querySelector('b')?.textContent)).toEqual(['0', '0', '0', '0']);
    expect(screen.queryByText(/بلا جهاز مناسب|غير متصل/)).not.toBeInTheDocument();
  });

  it('leaves out the runners number for a role that cannot read runners', () => {
    mount({ ...FULL, Runners: null, Invitations: null });
    expect(screen.getAllByRole('link')).toHaveLength(3);
    expect(screen.queryByText(Ar['home.stat.online'])).not.toBeInTheDocument();
  });

  it('adds a tile for the deploys running now, with the failed ones as its note', async () => {
    const { links, user } = mount({ ...FULL, Deployments: { Running: 2, FailedLast24h: 3, RollbackFailed: 0, Pending: 0 } });
    const tiles = screen.getAllByRole('link');
    expect(tiles).toHaveLength(5);
    const deploys = tiles.at(-1);
    if (deploys === undefined) throw new Error('no deploys tile');
    expect(deploys).toHaveAttribute('href', '/go/deploys');
    expect(deploys.querySelector('b')?.textContent).toBe('2');
    expect(deploys).toHaveTextContent(Ar['home.stat.deploys.failed'].replace('{count}', isolate('3')));
    await user.click(deploys);
    expect(links.open).toHaveBeenCalledWith('deploys');
  });
});

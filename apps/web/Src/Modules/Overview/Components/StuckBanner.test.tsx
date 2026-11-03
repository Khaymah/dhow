import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import type { TOverviewJobs } from '@/Modules/Overview/Api/OverviewApi';
import { StuckBanner } from '@/Modules/Overview/Components/StuckBanner';
import type { IOverviewLinks } from '@/Modules/Overview/Types/OverviewTargets';
import { isolate } from '@/I18n/Translate';

const jobs = (stuck: number): TOverviewJobs => ({ Running: 0, Queued: stuck + 1, QueuedNoRunner: stuck, FailedLast24h: 0 });

const mount = (value: TOverviewJobs | null, canAddRunner = true) => {
  const links: IOverviewLinks = { href: (target) => `/${target}`, open: vi.fn() };
  return { ...renderWithProviders(<StuckBanner jobs={value} canAddRunner={canAddRunner} links={links} />), links };
};

describe('StuckBanner', () => {
  it('stays hidden while nothing is stuck, or the jobs are unknown', () => {
    mount(jobs(0));
    mount(null);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it.each([
    [1, 'تشغيل واحد ينتظر ولا يوجد جهاز تشغيل مناسب له'],
    [2, 'تشغيلان ينتظران ولا يوجد جهاز تشغيل مناسب لهما'],
    [3, `${isolate('3')} تشغيلات تنتظر ولا يوجد جهاز تشغيل مناسب لها`],
    [12, `${isolate('12')} تشغيلًا تنتظر ولا يوجد جهاز تشغيل مناسب لها`],
    [102, `${isolate('102')} تشغيل ينتظر ولا يوجد جهاز تشغيل مناسب له`],
  ])('says how many jobs wait (%i)', (count, title) => {
    mount(jobs(count));
    expect(screen.getByRole('status')).toHaveTextContent(title);
  });

  it('sends a runner manager to add a runner', async () => {
    const { links, user } = mount(jobs(3));
    expect(screen.getByRole('status')).toHaveTextContent(Ar['home.stuck.fix.manager']);
    await user.click(screen.getByRole('button', { name: Ar['home.stuck.addRunner'] }));
    expect(links.open).toHaveBeenCalledWith('addRunner');
  });

  it('sends anyone else to the waiting jobs', async () => {
    const { links, user } = mount(jobs(3), false);
    expect(screen.getByRole('status')).toHaveTextContent(Ar['home.stuck.fix.member']);
    expect(screen.queryByRole('button', { name: Ar['home.stuck.addRunner'] })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['home.stuck.showJobs'] }));
    expect(links.open).toHaveBeenCalledWith('stuck');
  });
});

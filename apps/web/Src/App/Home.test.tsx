import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { overviewFixture, startBackend, type IOverviewFixture } from '@/Core/Testing/Backend';
import { gate, type TGateMode } from '@/Core/Testing/Gate';
import { jobFixture, jobsRoute } from '@/Core/Testing/JobsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const BUSY = overviewFixture({
  Jobs: { Running: 1, Queued: 4, QueuedNoRunner: 3, FailedLast24h: 2 },
  Runners: { Online: 2, Offline: 1 },
  Pending: 2,
});

const mount = async (options: { overview?: (role: string) => IOverviewFixture; role?: string; mode?: TGateMode; url?: string } = {}) => {
  const overviewGate = gate(/\/overview$/, options.mode ?? 'pass');
  const backend = startBackend({
    role: options.role,
    overview: options.overview,
    routes: [overviewGate.route, jobsRoute({ jobs: [jobFixture({ Name: 'deploy-staging', QueuedReason: 'NO_MATCHING_RUNNER' })] })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [options.url ?? '/'] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  await screen.findByRole('heading', { level: 1, name: Ar['home.title'] });
  return { ...backend, router, user, overviewGate };
};

const main = () => screen.getByRole('main');
const menu = () => screen.getByRole('navigation', { name: Ar['shell.nav.label'] });
const bottom = () => screen.getByRole('navigation', { name: Ar['shell.bottom.label'] });
const menuLink = (label: string) => within(menu()).getByRole('link', { name: new RegExp(`^${label}`) });

describe('the home page', () => {
  it('shows a skeleton for the numbers while the overview loads', async () => {
    await mount({ mode: 'hang' });
    expect(await screen.findByRole('status', { name: Ar['home.overview.loading'] })).toBeInTheDocument();
    expect(within(main()).queryByRole('link', { name: new RegExp(Ar['home.stat.running']) })).not.toBeInTheDocument();
  });

  it('shows an error with a retry, then the numbers', async () => {
    const { overviewGate, user } = await mount({ mode: 'fail', overview: BUSY });
    expect((await screen.findAllByText(Ar['home.overview.error'])).length).toBeGreaterThan(0);
    overviewGate.set('pass');
    await user.click(screen.getAllByRole('button', { name: Ar['jobs.retry'] })[0] ?? document.body);
    expect(await within(main()).findByRole('link', { name: new RegExp(Ar['home.stat.running']) })).toBeInTheDocument();
  });

  it('is calm on a fresh organization: zeros, no banner, no counters', async () => {
    await mount();
    const running = await within(main()).findByRole('link', { name: new RegExp(Ar['home.stat.running']) });
    expect(running).toHaveTextContent('0');
    expect(await screen.findByText(Ar['home.attention.calm'])).toBeInTheDocument();
    expect(screen.queryByText(Ar['home.stuck.fix.manager'])).not.toBeInTheDocument();
    expect(within(menu()).getAllByRole('link').filter((link) => link.querySelector('[aria-label]') !== null)).toEqual([]);
    expect(bottom().querySelector('[data-dot]')).toBeNull();
  });

  it('shows the stuck banner, the numbers and the counters when there is work', async () => {
    await mount({ overview: BUSY });
    const banner = await screen.findByText(arText('home.stuck.title', { count: 3 }));
    expect(banner.closest('[role="status"]')).toHaveTextContent(Ar['home.stuck.fix.manager']);
    expect(within(main()).getByRole('link', { name: new RegExp(Ar['home.stat.queued']) })).toHaveTextContent('4');

    const jobs = await within(menu()).findByLabelText(Ar['shell.count.jobsStuck'].replace('{count}', isolate('4')).replace('{stuck}', isolate('3')));
    expect(jobs).toHaveTextContent('4');
    expect(jobs).toHaveAttribute('data-tone', 'warn');
    expect(within(menu()).getByLabelText(Ar['shell.count.runners'].replace('{count}', isolate('1')))).toHaveTextContent('1');
    expect(within(menu()).getByLabelText(arText('shell.count.invitations', { count: 2 }))).toHaveTextContent('2');
    expect(within(bottom()).getByRole('link', { name: new RegExp(Ar['shell.nav.jobs']) }).querySelector('[data-dot="warn"]')).not.toBeNull();
    expect(menuLink(Ar['shell.nav.home']).querySelector('[aria-label]')).toBeNull();
  });

  it('keeps the plain tone and no phone dot when queued jobs all have a runner', async () => {
    await mount({ overview: overviewFixture({ Jobs: { Queued: 2 } }) });
    const jobs = await within(menu()).findByLabelText(Ar['shell.count.jobs'].replace('{count}', isolate('2')));
    expect(jobs).not.toHaveAttribute('data-tone');
    expect(bottom().querySelector('[data-dot]')).toBeNull();
  });

  it('hides the counters a role may not read', async () => {
    await mount({ overview: BUSY, role: 'developer' });
    await within(menu()).findByLabelText(Ar['shell.count.runners'].replace('{count}', isolate('1')));
    expect(within(menu()).queryByLabelText(arText('shell.count.invitations', { count: 2 }))).not.toBeInTheDocument();
  });

  it('gives a viewer the jobs numbers only and sends them to the waiting jobs', async () => {
    const { user, router } = await mount({ overview: BUSY, role: 'viewer' });
    expect(await screen.findByText(Ar['home.stuck.fix.member'])).toBeInTheDocument();
    expect(within(main()).queryByRole('link', { name: new RegExp(Ar['home.stat.online']) })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: Ar['home.runners.title'] })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: Ar['home.events.title'] })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['home.stuck.showJobs'] }));
    await waitFor(() => {
      expect(router.state.location.href).toBe('/jobs?state=queued');
    });
  });

  it('opens both failed states from the failed tile, and still opens a single state', async () => {
    const { user, router, calls } = await mount({ overview: BUSY });
    const failed = await within(main()).findByRole('link', { name: new RegExp(Ar['home.stat.failed']) });
    expect(failed).toHaveAttribute('href', '/jobs?state=failed,timed_out');
    await user.click(failed);
    await screen.findByRole('heading', { level: 1, name: Ar['jobs.title'] });
    expect(router.state.location.search).toEqual({ state: 'failed,timed_out' });
    const filter = screen.getByRole('group', { name: Ar['jobs.filter.label'] });
    expect(within(filter).getByRole('button', { name: Ar['jobs.state.failed'] })).toHaveAttribute('aria-pressed', 'true');
    expect(within(filter).getByRole('button', { name: Ar['jobs.state.timed_out'] })).toHaveAttribute('aria-pressed', 'true');
    expect(within(filter).getByRole('button', { name: Ar['jobs.filter.all'] })).toHaveAttribute('aria-pressed', 'false');
    await waitFor(() => {
      const lists = calls.map((call) => new URL(call.url)).filter((url) => url.pathname.endsWith('/jobs'));
      expect(lists.some((url) => url.searchParams.getAll('State').join() === 'failed,timed_out')).toBe(true);
    });

    await user.click(within(filter).getByRole('button', { name: Ar['jobs.state.failed'] }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ state: 'timed_out' });
    });
  });

  it('opens the jobs page filtered from a number', async () => {
    const { user, router } = await mount({ overview: BUSY });
    await user.click(await within(main()).findByRole('link', { name: new RegExp(Ar['home.stat.queued']) }));
    await screen.findByRole('heading', { level: 1, name: Ar['jobs.title'] });
    expect(router.state.location.href).toBe('/jobs?state=queued');
    const filter = screen.getByRole('group', { name: Ar['jobs.filter.label'] });
    expect(within(filter).getByRole('button', { name: Ar['jobs.state.queued'] })).toHaveAttribute('aria-pressed', 'true');
    await user.click(within(filter).getByRole('button', { name: Ar['jobs.filter.all'] }));
    await waitFor(() => {
      expect(router.state.location.href).toBe('/jobs');
    });
  });
});

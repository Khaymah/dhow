import { createMemoryHistory } from '@tanstack/react-router';
import { render, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { backendFailure, startBackend } from '@/Core/Testing/Backend';
import { jobFixture, jobsFixture, jobsRoute, type IJobsBackend } from '@/Core/Testing/JobsBackend';
import { findPage } from '@/Core/Testing/Pages';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { JOBS_GUIDE_URL } from '@/Modules/Jobs/Types/JobStates';
import { isolate } from '@/I18n/Translate';

const mount = async (jobs: IJobsBackend, role = 'owner') => {
  const backend = startBackend({ role, routes: [jobsRoute(jobs)] });
  const router = createAppRouter(createMemoryHistory({ initialEntries: ['/jobs'] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = await findPage(Ar['jobs.title']);
  return { ...backend, router, user, dialog };
};

const listCalls = (calls: readonly Request[]) =>
  calls.map((call) => new URL(call.url)).filter((url) => url.pathname.endsWith('/jobs'));

describe('the jobs page', () => {
  it('says how jobs get created when there are none', async () => {
    const { dialog } = await mount({ jobs: [] });
    expect(await within(dialog).findByText(Ar['jobs.empty.title'])).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['jobs.empty.about'])).toBeInTheDocument();
    const guide = within(dialog).getByRole('link', { name: Ar['jobs.empty.guide'] });
    expect(guide).toHaveAttribute('href', JOBS_GUIDE_URL.ar);
    expect(guide).toHaveAttribute('rel', 'noreferrer');
    expect(dialog.textContent).not.toMatch(/[0-9a-f]{8}-[0-9a-f]{4}-/);
    expect(dialog.textContent).not.toContain('/api/v1');
  });

  it('shows a skeleton while the first page loads', async () => {
    let release: (() => void) | undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const jobs: IJobsBackend = { jobs: jobsFixture(2) };
    jobs.list = async (request, url) => {
      await gate;
      jobs.list = undefined;
      return jobsRoute(jobs)(request, url) ?? backendFailure('NOT_FOUND', 'x', 404);
    };
    const { dialog } = await mount(jobs);
    expect(await within(dialog).findByRole('status', { name: Ar['jobs.loading'] })).toBeInTheDocument();
    release?.();
    expect(await within(dialog).findByRole('list', { name: Ar['jobs.list.title'] })).toBeInTheDocument();
  });

  it('shows an error with a retry when the list cannot be loaded', async () => {
    let failing = true;
    const jobs: IJobsBackend = { jobs: jobsFixture(1) };
    jobs.list = () => (failing ? backendFailure('INTERNAL', 'حدث خطأ في السيرفر.', 500) : undefined);
    const { dialog, user } = await mount(jobs);
    expect(await within(dialog).findByText(Ar['jobs.error.title'])).toBeInTheDocument();
    failing = false;
    await user.click(within(dialog).getByRole('button', { name: Ar['jobs.retry'] }));
    expect(await within(dialog).findByRole('list', { name: Ar['jobs.list.title'] })).toBeInTheDocument();
  });

  it('lists jobs with their state, reason, timing and attempts', async () => {
    const { dialog } = await mount({ jobs: jobsFixture(3) });
    const list = await within(dialog).findByRole('list', { name: Ar['jobs.list.title'] });
    const rows = within(list).getAllByRole('listitem');
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent('job-1');
    expect(rows[0]).toHaveTextContent(Ar['jobs.state.succeeded']);
    expect(rows[0]).toHaveTextContent(Ar['jobs.row.took'].replace('{duration}', isolate(Ar['jobs.duration.minutes'].replace('{m}', isolate('1')).replace('{s}', isolate('0')))));
    expect(rows[1]).toHaveTextContent(Ar['jobs.state.failed']);
    expect(rows[1]).toHaveTextContent(Ar['jobs.reason.STEP_FAILED']);
    expect(rows[1]).toHaveTextContent(arText('jobs.row.attempts', { count: 1 }));
  });

  it('makes queued jobs without a runner obvious and filters to them', async () => {
    const stuck = [1, 2, 3].map((n) =>
      jobFixture({ Id: `0198a000-0000-7000-8000-00000000010${String(n)}`, Name: `stuck-${String(n)}`, QueuedReason: 'NO_MATCHING_RUNNER' }),
    );
    const { dialog, user, calls } = await mount({ jobs: [...stuck, ...jobsFixture(2)] });
    expect(await within(dialog).findByText(arText('jobs.stuck.title', { count: 3 }))).toBeInTheDocument();
    const list = await within(dialog).findByRole('list', { name: Ar['jobs.list.title'] });
    expect(within(list).getAllByText(Ar['jobs.stuck.badge'])).toHaveLength(3);

    await user.click(within(dialog).getByRole('button', { name: Ar['jobs.stuck.show'] }));
    expect(within(dialog).getByRole('button', { name: Ar['jobs.state.queued'] })).toHaveAttribute('aria-pressed', 'true');
    await waitFor(() => {
      expect(within(dialog).getByText(arText('jobs.list.total', { count: 3 }))).toBeInTheDocument();
    });
    expect(listCalls(calls).some((url) => url.searchParams.get('State') === 'queued' && url.searchParams.get('Page') === '1')).toBe(true);
  });

  it('pages through the list and says when a filter matches nothing', async () => {
    const { dialog, user, calls } = await mount({ jobs: jobsFixture(25) });
    await within(dialog).findByRole('list', { name: Ar['jobs.list.title'] });
    await user.click(within(dialog).getByRole('button', { name: Ar['pager.next'] }));
    await waitFor(() => {
      expect(within(dialog).getByText(Ar['pager.status'].replace('{page}', isolate('2')).replace('{pages}', isolate('2')))).toBeInTheDocument();
    });
    expect(listCalls(calls).map((url) => url.searchParams.get('Limit'))).toContain('20');

    await user.click(within(dialog).getByRole('button', { name: Ar['jobs.state.lost'] }));
    expect(await within(dialog).findByText(Ar['jobs.empty.filtered'])).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: Ar['jobs.empty.showAll'] }));
    expect(await within(dialog).findByRole('list', { name: Ar['jobs.list.title'] })).toBeInTheDocument();
  });

  it('opens a job as its own page', async () => {
    const { dialog, user, router } = await mount({ jobs: jobsFixture(1) });
    const list = await within(dialog).findByRole('list', { name: Ar['jobs.list.title'] });
    await user.click(within(list).getByRole('button', { name: /job-1/ }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/jobs/0198a000-0000-7000-8000-000000000000');
    });
  });
});

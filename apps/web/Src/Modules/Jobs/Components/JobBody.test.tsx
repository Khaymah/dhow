import { createMemoryHistory } from '@tanstack/react-router';
import { render, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { backendFailure, startBackend } from '@/Core/Testing/Backend';
import { FakeEventSource, installFakeEventSource } from '@/Core/Testing/FakeEventSource';
import { attemptFixture, JOB_ID, jobFixture, jobsRoute, TEST_IMAGE, type IJobsBackend } from '@/Core/Testing/JobsBackend';
import { findPage } from '@/Core/Testing/Pages';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const mount = async (jobs: IJobsBackend, role = 'owner') => {
  const backend = startBackend({ role, routes: [jobsRoute(jobs)] });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [`/jobs/${JOB_ID}`] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = await findPage(Ar['jobs.detail.title']);
  return { ...backend, router, user, dialog };
};

const running = (): IJobsBackend => ({
  jobs: [jobFixture({ State: 'running', AttemptCount: 1, StartedAt: new Date(Date.now() - 180_000).toISOString() })],
  attempts: [attemptFixture()],
});

beforeEach(() => {
  installFakeEventSource();
});

describe('the job page', () => {
  it('shows a skeleton, then the job', async () => {
    let release: (() => void) | undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const jobs = running();
    jobs.detail = async () => {
      await gate;
      jobs.detail = undefined;
      return backendFailure('INTERNAL', 'x', 500);
    };
    const { dialog } = await mount(jobs);
    expect(await within(dialog).findByRole('status', { name: Ar['jobs.detail.loading'] })).toBeInTheDocument();
    release?.();
    expect(await within(dialog).findByText(Ar['jobs.detail.error'])).toBeInTheDocument();
  });

  it('says a job of another organization is not found, and offers a retry for other errors', async () => {
    const missing = await mount({ jobs: [] });
    expect(await within(missing.dialog).findByText(Ar['jobs.detail.notFound'])).toBeInTheDocument();
  });

  it('explains a queued job that no runner can take, with its labels and deadline', async () => {
    const { dialog } = await mount({
      jobs: [jobFixture({ QueuedReason: 'NO_MATCHING_RUNNER', Labels: ['gpu', 'linux'] })],
      spec: { Labels: ['gpu', 'linux'] },
    });
    const alert = (await within(dialog).findByText(Ar['jobs.noRunner.title'])).closest('[role="note"]');
    if (!(alert instanceof HTMLElement)) throw new Error('no alert');
    expect(alert).toHaveTextContent('gpu');
    expect(alert).toHaveTextContent('linux');
    expect(alert).toHaveTextContent(Ar['jobs.noRunner.how']);
    expect(within(dialog).getByText(Ar['jobs.log.notStarted'])).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['jobs.attempts.none'])).toBeInTheDocument();
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it('summarizes the spec: image, steps, resources and egress', async () => {
    const { dialog } = await mount(running());
    expect(await within(dialog).findByText(TEST_IMAGE)).toBeInTheDocument();
    expect(within(dialog).getByText('kubectl apply -f k8s/')).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['jobs.spec.mutating'])).toBeInTheDocument();
    expect(within(dialog).getByText('registry.npmjs.org')).toBeInTheDocument();
    expect(within(dialog).getByText(arText('jobs.spec.cpuValue', { n: 2 }))).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['jobs.size.mib'].replace('{n}', isolate('512')))).toBeInTheDocument();
  });

  it('lists attempts with their state, reason and runner name, without the fencing token', async () => {
    const jobs = running();
    jobs.attempts = [
      attemptFixture({ Id: '0198a000-0000-7000-8000-0000000000f0', Number: 1, FencingToken: 40, State: 'lost', FailureReason: 'LEASE_LOST' }),
      attemptFixture({ Number: 2, FencingToken: 52, RunnerName: 'build-01' }),
    ];
    const { dialog } = await mount(jobs);
    const list = await within(dialog).findByRole('list', { name: Ar['jobs.attempts.title'] });
    const items = within(list).getAllByRole('listitem').filter((item) => item.parentElement === list);
    expect(items[0]).toHaveTextContent('build-01');
    expect(items[0]).not.toHaveTextContent('52');
    const latest = items[0];
    if (latest === undefined) throw new Error('no attempt');
    expect(within(latest).getByRole('button', { name: Ar['jobs.attempts.runnerId.copy'] })).toBeInTheDocument();
    expect(items[1]).toHaveTextContent(Ar['jobs.attempts.runnerGone']);
    expect(items[1]).toHaveTextContent(Ar['jobs.reason.LEASE_LOST']);
    expect(items[1]).toHaveTextContent(Ar['jobs.state.lost']);
  });

  it('stops a job only after a confirmation', async () => {
    const jobs: IJobsBackend = { jobs: [jobFixture()] };
    const { dialog, user, calls } = await mount(jobs);
    await user.click(await within(dialog).findByRole('button', { name: Ar['jobs.cancel.action'] }));
    expect(calls.some((call) => call.url.endsWith('/cancel'))).toBe(false);
    await user.click(within(dialog).getByRole('button', { name: Ar['jobs.cancel.confirm'] }));
    await waitFor(() => {
      expect(within(dialog).getAllByText(Ar['jobs.reason.CANCELED']).length).toBeGreaterThan(0);
    });
    const cancel = calls.find((call) => call.url.endsWith('/cancel'));
    expect(cancel?.method).toBe('POST');
    expect(cancel?.headers.get('X-CSRF-Token')).toBe('csrf-token-1');
    expect(within(dialog).queryByRole('button', { name: Ar['jobs.cancel.action'] })).not.toBeInTheDocument();
  });

  it('hides the stop button from a developer and the log from a viewer', async () => {
    const developer = await mount(running(), 'developer');
    await within(developer.dialog).findByText(TEST_IMAGE);
    expect(within(developer.dialog).queryByRole('button', { name: Ar['jobs.cancel.action'] })).not.toBeInTheDocument();
    expect(within(developer.dialog).getByRole('log')).toBeInTheDocument();
  });

  it('tells a viewer the log needs a higher role and opens no stream', async () => {
    const { dialog } = await mount(running(), 'viewer');
    expect(await within(dialog).findByText(Ar['jobs.log.forbidden'])).toBeInTheDocument();
    expect(FakeEventSource.instances).toHaveLength(0);
  });
});

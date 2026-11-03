import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend } from '@/Core/Testing/Backend';
import { JOB_ID, jobFixture, jobsRoute } from '@/Core/Testing/JobsBackend';
import { runnerFixture, runnerId, runnersRoute } from '@/Core/Testing/RunnersBackend';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

configure({ asyncUtilTimeout: 5000 });

const RUNNERS = [
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
];

const mount = (role: string, queuedReason?: string) => {
  const job = jobFixture({
    Labels: ['linux-builders', 'arm64'],
    ...(queuedReason === undefined ? {} : { QueuedReason: queuedReason }),
  });
  const backend = startBackend({
    role,
    routes: [jobsRoute({ jobs: [job] }), runnersRoute({ runners: RUNNERS })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [`/jobs/${JOB_ID}`] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const runnerCalls = () => backend.calls.filter((call) => /\/runners(\/|$)/.test(new URL(call.url).pathname));
  return { ...backend, router, user, runnerCalls };
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('why a stuck job is waiting, on the job page', () => {
  it('shows an owner the runners table and opens the paused runner', async () => {
    const view = mount('owner', 'NO_MATCHING_RUNNER');
    expect(await screen.findByText(Ar['jobs.noRunner.title'])).toBeInTheDocument();
    const table = await screen.findByRole('table', {
      name: Ar['runners.fit.table'],
    });
    expect(within(table).getAllByRole('row')).toHaveLength(3);
    await view.user.click(
      screen.getByRole('button', {
        name: Ar['runners.fit.resume'].replace('{name}', isolate('build-arm')),
      }),
    );
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(`/runners/${runnerId(2)}`);
    });
  });

  it('keeps the explanation for a viewer, without the table or a runners request', async () => {
    const view = mount('viewer', 'NO_MATCHING_RUNNER');
    expect(await screen.findByText(Ar['jobs.noRunner.title'])).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole('table', { name: Ar['runners.fit.table'] })).not.toBeInTheDocument();
    expect(view.runnerCalls()).toEqual([]);
  });

  it('asks for no runners when a runner can take the job', async () => {
    const view = mount('owner');
    expect(await screen.findByText(Ar['jobs.waiting.title'])).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByText(Ar['runners.fit.title'])).not.toBeInTheDocument();
    expect(view.runnerCalls()).toEqual([]);
  });
});

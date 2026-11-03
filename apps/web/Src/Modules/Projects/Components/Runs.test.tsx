import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { TRole } from '@/Core/Auth/AuthTypes';
import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch } from '@/Core/Testing/Fetch';
import {
  ARTIFACT_ID,
  artifactFixture,
  detailFixture,
  JOB_ID,
  ok,
  page,
  PROJECT_ID,
  RUN_ID,
  runFixture,
  runJobFixture,
} from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { RunBody } from '@/Modules/Projects/Components/RunBody';
import { RunsCard } from '@/Modules/Projects/Components/RunsCard';

const path = (request: Request) => new URL(request.url).pathname;

const mountList = (list: (url: URL) => Response | Promise<Response>) => {
  const { calls } = stubFetch((request) => (path(request).endsWith('/runs') ? list(new URL(request.url)) : backendFailure('NOT_FOUND', 'x', 404)));
  const onOpenRun = vi.fn();
  const view = renderWithProviders(
    <RunsCard organizationId={ORG_ID} projectId={PROJECT_ID} hrefFor={(id) => `/projects/${PROJECT_ID}/runs/${id}`} onOpenRun={onOpenRun} />,
  );
  return { ...view, calls, onOpenRun };
};

describe('the runs list', () => {
  it('covers loading, error, empty and a filtered empty list', async () => {
    const pending = mountList(() => new Promise<Response>(() => undefined));
    expect(await screen.findByRole('status', { name: Ar['runs.loading'] })).toBeInTheDocument();
    pending.unmount();

    const failing = mountList(() => backendFailure('INTERNAL', 'تعطل السيرفر.', 500));
    expect(await screen.findByText(Ar['runs.error'])).toBeInTheDocument();
    failing.unmount();

    const view = mountList(() => page([]));
    expect(await screen.findByText(Ar['runs.empty'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runs.filter.failed'] }));
    expect(await screen.findByText(Ar['runs.empty.filtered'])).toBeInTheDocument();
    const filtered = view.calls.map((call) => new URL(call.url).searchParams.getAll('State')).find((states) => states.length > 0);
    expect(filtered).toEqual(['failed', 'canceled']);
  });

  it('shows the number, ref, short commit, trigger, actor, duration and a human reason', async () => {
    const view = mountList(() =>
      page([runFixture(), runFixture({ Id: 'r2', Number: 8, State: 'failed', FailureReason: 'HOST_KEY_MISMATCH', IsFork: true, Trigger: 'pull_request' })]),
    );
    const link = await screen.findByRole('link', { name: new RegExp(arText('runs.row.number', { number: 7 })) });
    expect(link).toHaveAttribute('href', `/projects/${PROJECT_ID}/runs/${RUN_ID}`);
    expect(within(link).getByText('main')).toBeInTheDocument();
    expect(within(link).getByText('0123456')).toBeInTheDocument();
    expect(link).toHaveTextContent(Ar['runs.kind.push']);
    expect(link).toHaveTextContent(arText('runs.row.by', { actor: 'sara' }));
    expect(link).toHaveTextContent(arText('runs.row.took', { duration: arText('jobs.duration.minutes', { m: 1, s: 5 }) }));
    expect(screen.getByText(Ar['runs.reason.HOST_KEY_MISMATCH'])).toBeInTheDocument();
    expect(screen.queryByText('HOST_KEY_MISMATCH')).toBeNull();
    expect(screen.getByText(Ar['runs.fork'])).toBeInTheDocument();
    await view.user.click(link);
    expect(view.onOpenRun).toHaveBeenCalledWith(RUN_ID);
  });
});

interface IDetailBackend {
  detail?: () => Response | Promise<Response>;
  artifacts?: () => Response;
  action?: (name: string) => Response;
}

const mountDetail = (role: TRole, { detail = () => ok(detailFixture()), artifacts = () => ok([artifactFixture()]), action }: IDetailBackend = {}) => {
  const posts: string[] = [];
  stubFetch((request) => {
    const p = path(request);
    if (p.endsWith('/me')) return envelope(meBody(role));
    if (request.method === 'POST') {
      const name = p.split('/').pop() ?? '';
      posts.push(name);
      return action?.(name) ?? ok(detailFixture(runFixture({ State: 'running', FinishedAt: undefined })));
    }
    if (p.endsWith('/artifacts')) return artifacts();
    if (p.endsWith(`/runs/${RUN_ID}`)) return detail();
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onOpenJob = vi.fn();
  const onSecondFactor = vi.fn();
  const view = renderWithProviders(
    <RunBody
      organizationId={ORG_ID}
      projectId={PROJECT_ID}
      runId={RUN_ID}
      role={role}
      crumbKey="crumb"
      jobHref={(id) => `/jobs/${id}`}
      onOpenJob={onOpenJob}
      onSecondFactor={onSecondFactor}
    />,
  );
  return { ...view, posts, onOpenJob, onSecondFactor };
};

const DAG = detailFixture(runFixture({ State: 'failed', FailureReason: 'JOBS_FAILED' }), [
  runJobFixture({ Id: 'j1', Key: 'build (node=20)', JobId: 'build', Matrix: { node: '20' } }),
  runJobFixture({ Id: 'j2', Key: 'build (node=22)', JobId: 'build', Matrix: { node: '22' }, CurrentJobId: undefined, State: 'canceled' }),
  runJobFixture({ Id: 'j3', Key: 'test', JobId: 'test', Name: 'test', Needs: ['build'], State: 'failed', FailureReason: 'STEP_FAILED' }),
  runJobFixture({ Id: 'j4', Key: 'deploy', JobId: 'deploy', Name: 'deploy', Needs: ['test'], State: 'skipped', FailureReason: 'NEEDS_FAILED', CurrentJobId: undefined }),
]);

describe('a run', () => {
  it('covers loading, not found and error', async () => {
    const pending = mountDetail('viewer', { detail: () => new Promise<Response>(() => undefined) });
    expect(await screen.findByRole('status', { name: Ar['runs.detail.loading'] })).toBeInTheDocument();
    pending.unmount();
    const missing = mountDetail('viewer', { detail: () => backendFailure('NOT_FOUND', 'x', 404) });
    expect(await screen.findByText(Ar['runs.detail.notFound'])).toBeInTheDocument();
    missing.unmount();
    mountDetail('viewer', { detail: () => backendFailure('INTERNAL', 'x', 500) });
    expect(await screen.findByText(Ar['runs.detail.error'])).toBeInTheDocument();
  });

  it('lists the DAG by stage with matrix values, needs, reasons and links to the live log', async () => {
    const view = mountDetail('developer', { detail: () => ok(DAG) });
    const stages = await screen.findByRole('list', { name: Ar['runs.stages.title'] });
    const [first, second, third] = within(stages).getAllByRole('list');
    if (first === undefined || second === undefined || third === undefined) throw new Error('three stages expected');
    expect(first).toHaveAccessibleName(arText('runs.stage.title', { number: 1 }));
    expect(within(first).getByText('node=20')).toBeInTheDocument();
    expect(within(first).getByText('node=22')).toBeInTheDocument();
    expect(within(second).getByText('build')).toBeInTheDocument();
    expect(within(second).getByText(Ar['runs.reason.STEP_FAILED'])).toBeInTheDocument();
    expect(within(third).getByText(Ar['runs.reason.NEEDS_FAILED'])).toBeInTheDocument();
    expect(screen.getByText(Ar['runs.reason.JOBS_FAILED'])).toBeInTheDocument();
    const log = within(second).getByRole('link', { name: 'test' });
    expect(log).toHaveAttribute('href', `/jobs/${JOB_ID}`);
    await view.user.click(log);
    expect(view.onOpenJob).toHaveBeenCalledWith(JOB_ID);
    expect(within(third).queryByRole('link')).toBeNull();
  });

  it('shows a reason it does not know as a sentence with the code, never the code alone', async () => {
    mountDetail('viewer', { detail: () => ok(detailFixture(runFixture({ State: 'failed', FailureReason: 'BRAND_NEW_CODE' }), [])) });
    expect(await screen.findByText(arText('runs.reason.unknown', { code: 'BRAND_NEW_CODE' }))).toBeInTheDocument();
  });

  it('lets a developer rerun failed jobs but not cancel, and a viewer only read', async () => {
    const view = mountDetail('developer', { detail: () => ok(detailFixture(runFixture({ State: 'failed', FailureReason: 'JOBS_FAILED' }))) });
    await view.user.click(await screen.findByRole('button', { name: Ar['runs.actions.rerun.action'] }));
    await waitFor(() => {
      expect(view.posts).toEqual(['rerun-failed']);
    });
    view.unmount();

    mountDetail('viewer', { detail: () => ok(detailFixture(runFixture({ State: 'running', FinishedAt: undefined }))) });
    expect(await screen.findByText(Ar['runs.actions.forbidden'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['runs.actions.cancel.action'] })).toBeNull();
    expect(screen.queryByRole('heading', { name: Ar['artifacts.title'] })).toBeNull();
  });

  it('asks before canceling', async () => {
    const view = mountDetail('maintainer', { detail: () => ok(detailFixture(runFixture({ State: 'running', FinishedAt: undefined }))) });
    await view.user.click(await screen.findByRole('button', { name: Ar['runs.actions.cancel.action'] }));
    expect(view.posts).toEqual([]);
    expect(screen.getByText(Ar['runs.actions.cancel.question'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runs.actions.cancel.confirm'] }));
    await waitFor(() => {
      expect(view.posts).toEqual(['cancel']);
    });
  });

  it('approves a fork run after a confirmation, and sends the user to set up a second factor when needed', async () => {
    const fork = detailFixture(runFixture({ State: 'awaiting_approval', IsFork: true, Trigger: 'pull_request', StartedAt: undefined, FinishedAt: undefined }), []);
    const view = mountDetail('maintainer', {
      detail: () => ok(fork),
      action: () => backendFailure('SECOND_FACTOR_REQUIRED', 'فعّل الخطوة الثانية أولًا.', 403),
    });
    expect(await screen.findByText(Ar['runs.approval.title'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runs.actions.approve.action'] }));
    expect(screen.getByText(Ar['runs.actions.approve.question'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runs.actions.approve.confirm'] }));
    expect(await screen.findByText(Ar['runs.actions.secondFactor.title'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runs.actions.secondFactor.action'] }));
    expect(view.onSecondFactor).toHaveBeenCalledOnce();
    expect(view.posts).toEqual(['approve']);
    view.unmount();

    mountDetail('developer', { detail: () => ok(fork) });
    expect(await screen.findByText(Ar['runs.actions.approve.forbidden'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['runs.actions.approve.action'] })).toBeNull();
  });

});

describe('the artifacts and problems of a run', () => {
  it('lists artifacts with size, expiry, untrusted mark and a REST download', async () => {
    mountDetail('developer', { artifacts: () => ok([artifactFixture({ Untrusted: true })]) });
    const download = await screen.findByRole('link', { name: Ar['artifacts.download'] });
    expect(download).toHaveAttribute('href', `/api/v1/organizations/${ORG_ID}/projects/${PROJECT_ID}/artifacts/${ARTIFACT_ID}/download`);
    expect(download).toHaveAttribute('download');
    expect(screen.getByText('web-dist')).toBeInTheDocument();
    expect(screen.getByText(Ar['artifacts.untrusted'])).toBeInTheDocument();
    expect(screen.getByText(Ar['artifacts.untrusted.about'])).toBeInTheDocument();
    expect(screen.getByText(new RegExp(arText('jobs.size.mib', { n: '2.5' })))).toBeInTheDocument();
  });

  it('says when a finished run has no artifacts, and when loading them failed', async () => {
    const view = mountDetail('developer', { artifacts: () => ok([]) });
    expect(await screen.findByText(Ar['artifacts.empty'])).toBeInTheDocument();
    view.unmount();
    mountDetail('developer', { artifacts: () => backendFailure('INTERNAL', 'x', 500) });
    expect(await screen.findByText(Ar['artifacts.error'])).toBeInTheDocument();
  });

  it('lists the problems of a wrong pipeline file', async () => {
    const issues = [{ Code: 'UNKNOWN_KEY', Severity: 'error', Path: 'jobs.build.imag', Line: 4, Column: 5, Message: 'unknown key' }];
    mountDetail('viewer', { detail: () => ok(detailFixture(runFixture({ State: 'failed', FailureReason: 'PIPELINE_INVALID' }), [], issues)) });
    expect(await screen.findByText(Ar['runs.reason.PIPELINE_INVALID'])).toBeInTheDocument();
    expect(screen.getByText(arText('runs.issues.where', { path: 'jobs.build.imag', line: 4, column: 5 }))).toBeInTheDocument();
    expect(screen.getByText('UNKNOWN_KEY: unknown key')).toBeInTheDocument();
  });
});

import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { approvalFixture, ENVIRONMENT_ID, gatedJob, newApprovalsState } from '@/Core/Testing/ApprovalsBackend';
import { mountRun } from '@/Core/Testing/ApprovalsHarness';
import { findPage } from '@/Core/Testing/Pages';
import { runJobFixture } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { formatDateTime } from '@/Core/Format/TimeFormat';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const HOUR = 3_600_000;
const TITLE = arText('runs.row.number', { number: 7 });

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

const gateOf = async (kind: string): Promise<HTMLElement> => {
  const main = await findPage(TITLE);
  await waitFor(() => {
    expect(main.querySelector(`[data-gate="${kind}"]`)).not.toBeNull();
  });
  return main.querySelector<HTMLElement>(`[data-gate="${kind}"]`) ?? main;
};

describe('a job that waits at the environment gate', () => {
  it('says that a ref the environment does not allow was skipped, and does not blame the run', async () => {
    mountRun({ jobs: [gatedJob('', { State: 'skipped', Gate: undefined, FailureReason: 'ENVIRONMENT_REF_NOT_ALLOWED' })] });
    const gate = await gateOf('skipped');
    expect(within(gate).getByText(Ar['approvals.gate.skipped.badge'])).toBeInTheDocument();
    expect(within(gate).getByText(arText('approvals.gate.skipped.body', { ref: 'main', environment: 'production' }))).toBeInTheDocument();
    expect(within(gate).getByText(Ar['approvals.gate.skipped.hint'])).toBeInTheDocument();
    expect(screen.queryByText(Ar['runs.reason.ENVIRONMENT_REF_NOT_ALLOWED'])).not.toBeInTheDocument();
  });

  it('links the skipped job to the environment page for people who manage environments only', async () => {
    const owner = mountRun({ jobs: [gatedJob('', { State: 'skipped', Gate: undefined, FailureReason: 'ENVIRONMENT_REF_NOT_ALLOWED' })] });
    const link = await within(await gateOf('skipped')).findByRole('link', { name: Ar['approvals.gate.openEnvironment'] });
    expect(link).toHaveAttribute('href', expect.stringMatching(/\/environments\/production$/));
    await owner.user.click(link);
    await findPage('production');
  });

  it('hides that link from a developer', async () => {
    mountRun({ role: 'developer', jobs: [gatedJob('', { State: 'skipped', Gate: undefined, FailureReason: 'ENVIRONMENT_REF_NOT_ALLOWED' })] });
    const gate = await gateOf('skipped');
    expect(within(gate).queryByRole('link')).not.toBeInTheDocument();
  });

  it('shows the wait timer as a local time', async () => {
    const opens = new Date(Date.now() + 2 * HOUR).toISOString();
    mountRun({ jobs: [gatedJob('timer', { GateOpensAt: opens })] });
    const gate = await gateOf('timer');
    expect(within(gate).getByText(Ar['approvals.gate.timer.badge'])).toBeInTheDocument();
    expect(within(gate).getByText(arText('approvals.gate.timer.body', { time: formatDateTime(opens, 'ar') }))).toBeInTheDocument();
  });

  it('says that another run holds the environment', async () => {
    mountRun({ jobs: [gatedJob('environment_busy')] });
    const gate = await gateOf('environment_busy');
    expect(within(gate).getByText(Ar['approvals.gate.busy.badge'])).toBeInTheDocument();
    expect(within(gate).getByText(arText('approvals.gate.busy.body', { environment: 'production' }))).toBeInTheDocument();
  });

  it('says that deploys are paused, with the link for an admin', async () => {
    mountRun({ jobs: [gatedJob('paused')] });
    const gate = await gateOf('paused');
    expect(within(gate).getByText(Ar['approvals.gate.paused.badge'])).toBeInTheDocument();
    expect(within(gate).getByText(arText('approvals.gate.paused.body', { environment: 'production' }))).toBeInTheDocument();
    expect(within(gate).getByRole('link', { name: Ar['approvals.gate.openEnvironment'] })).toBeInTheDocument();
  });

});

describe('an approval request at the gate', () => {
  it('shows since when the approval waits and when the request expires', async () => {
    const state = newApprovalsState();
    const expires = new Date(Date.now() + 40 * HOUR).toISOString();
    state.approvals = [approvalFixture({ CreatedAt: new Date(Date.now() - 2 * HOUR).toISOString(), ExpiresAt: expires })];
    mountRun({ state });
    const gate = await gateOf('approval');
    expect(within(gate).getByText(Ar['approvals.gate.approval.badge'])).toBeInTheDocument();
    expect(await within(gate).findByText(arText('approvals.gate.approval.since', { time: arText('time.hours', { count: 2 }) }))).toBeInTheDocument();
    expect(within(gate).getByText(arText('approvals.gate.approval.expires', { time: formatDateTime(expires, 'ar') }))).toBeInTheDocument();
  });

  it('shows one set of decisions for two jobs that wait on the same environment', async () => {
    mountRun({
      jobs: [gatedJob('approval'), gatedJob('approval', { Id: '0198a000-0000-7000-8000-000000000803', Key: 'migrate', JobId: 'migrate', Name: 'migrate' })],
    });
    await gateOf('approval');
    await waitFor(() => {
      expect(screen.getAllByText(Ar['approvals.gate.approval.badge'])).toHaveLength(2);
    });
    expect(await screen.findAllByRole('button', { name: Ar['approvals.gate.approve'] })).toHaveLength(1);
    expect(screen.getAllByText(Ar['approvals.gate.approval.shared'])).toHaveLength(1);
  });

  it('asks for the approvals again when they fail to load, and shows the retry', async () => {
    const state = newApprovalsState();
    state.failList = true;
    const view = mountRun({ state });
    const gate = await gateOf('approval');
    expect(await within(gate).findByText(Ar['approvals.gate.error'])).toBeInTheDocument();
    state.failList = false;
    await view.user.click(within(gate).getByRole('button', { name: Ar['status.retry'] }));
    expect(await within(gate).findByRole('button', { name: Ar['approvals.gate.approve'] })).toBeInTheDocument();
  });

  it('shows a loading state while the approvals arrive', async () => {
    const state = newApprovalsState();
    state.hangList = true;
    mountRun({ state });
    const gate = await gateOf('approval');
    expect(await within(gate).findByRole('status', { name: Ar['approvals.gate.loading'] })).toBeInTheDocument();
    expect(within(gate).queryByRole('button', { name: Ar['approvals.gate.approve'] })).not.toBeInTheDocument();
  });

  it('writes the plain reason of a rejected, expired or replaced deploy', async () => {
    mountRun({
      jobs: [
        gatedJob('', { State: 'failed', Gate: undefined, FailureReason: 'DEPLOYMENT_REJECTED' }),
        runJobFixture({ Id: 'j2', Key: 'a', JobId: 'a', Name: 'a', State: 'failed', FailureReason: 'APPROVAL_EXPIRED' }),
        runJobFixture({ Id: 'j3', Key: 'b', JobId: 'b', Name: 'b', State: 'canceled', FailureReason: 'SUPERSEDED' }),
      ],
    });
    const main = await findPage(TITLE);
    expect(await within(main).findByText(Ar['runs.reason.DEPLOYMENT_REJECTED'])).toBeInTheDocument();
    expect(within(main).getByText(Ar['runs.reason.APPROVAL_EXPIRED'])).toBeInTheDocument();
    expect(within(main).getByText(Ar['runs.reason.SUPERSEDED'])).toBeInTheDocument();
    expect(main.querySelector('[data-gate]')).toBeNull();
  });

  it('lists every approval request of the run with its decision, comment and reason', async () => {
    const state = newApprovalsState();
    state.approvals = [
      approvalFixture({ State: 'approved', Comment: 'Checked the migration', DecidedAt: new Date(Date.now() - HOUR).toISOString(), FactorKind: 'totp' }),
      approvalFixture({ Id: '0198a000-0000-7000-8000-000000000d09', Round: 2, State: 'bypassed', BypassReason: 'Security fix', EnvironmentId: ENVIRONMENT_ID }),
    ];
    mountRun({ state });
    const main = await findPage(TITLE);
    const list = await within(main).findByRole('list', { name: Ar['approvals.card.title'] });
    expect(within(list).getByText(Ar['approvals.state.approved'])).toBeInTheDocument();
    expect(within(list).getByText(Ar['approvals.state.bypassed'])).toBeInTheDocument();
    expect(within(list).getByText('Checked the migration')).toBeInTheDocument();
    expect(within(list).getByText('Security fix')).toBeInTheDocument();
    expect(within(list).getByText(arText('approvals.row.factor', { factor: Ar['approvals.factor.totp'] }), { exact: false })).toBeInTheDocument();
  });
});

import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import {
  APPROVAL_ID,
  approvalFixture,
  gatedJob,
  GOOD_CODE,
  newApprovalsState,
  OTHER_APPROVAL_ID,
  type IApprovalsState,
} from '@/Core/Testing/ApprovalsBackend';
import { mountRun } from '@/Core/Testing/ApprovalsHarness';
import { backendFailure } from '@/Core/Testing/Backend';
import { findPage } from '@/Core/Testing/Pages';
import { detailFixture, PROJECT_ID, RUN_ID, runFixture } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const TITLE = arText('runs.row.number', { number: 7 });
const API = '/organizations';

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

type TView = ReturnType<typeof mountRun>;

const openSlide = async (view: TView, button: string, title: string): Promise<HTMLElement> => {
  const main = await findPage(TITLE);
  await view.user.click(await within(main).findByRole('button', { name: button }));
  return screen.findByRole('dialog', { name: title });
};

const enterCode = async (view: TView, dialog: HTMLElement, code = GOOD_CODE) => {
  await view.user.type(await within(dialog).findByLabelText(Ar['factor.method.totp']), code);
};

describe('who sees the decisions', () => {
  it('shows approve and reject to a reviewer, and keeps them from everyone else', async () => {
    const reviewer = mountRun({ role: 'developer' });
    const main = await findPage(TITLE);
    expect(await within(main).findByRole('button', { name: Ar['approvals.gate.approve'] })).toBeInTheDocument();
    expect(within(main).getByRole('button', { name: Ar['approvals.gate.reject'] })).toBeInTheDocument();
    expect(reviewer.state.mineQueries).toEqual(['?Mine=true']);
  });

  it('keeps approve and reject from a developer who is not a reviewer of the environment', async () => {
    const state = newApprovalsState();
    state.mine = [];
    mountRun({ role: 'developer', state });
    const main = await findPage(TITLE);
    expect(await within(main).findByText(arText('approvals.gate.approval.reviewers', { environment: 'production' }))).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['approvals.gate.approve'] })).not.toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['approvals.gate.bypass'] })).not.toBeInTheDocument();
  });

  it('shows a viewer the wait and no button, and asks nothing about the requests of others', async () => {
    const view = mountRun({ role: 'viewer' });
    const main = await findPage(TITLE);
    expect(await within(main).findByText(arText('approvals.gate.approval.reviewers', { environment: 'production' }))).toBeInTheDocument();
    expect(within(main).queryAllByRole('button').map((button) => button.textContent)).not.toContain(Ar['approvals.gate.approve']);
    expect(view.state.mineQueries).toEqual([]);
  });

  it.each([
    ['owner', true],
    ['admin', true],
    ['maintainer', false],
    ['developer', false],
  ] as const)('offers the bypass to a %s: %s', async (role, bypassShown) => {
    mountRun({ role });
    const main = await findPage(TITLE);
    await within(main).findByText(Ar['approvals.gate.approval.badge']);
    if (bypassShown) {
      expect(await within(main).findByRole('button', { name: Ar['approvals.gate.bypass'] })).toBeInTheDocument();
      return;
    }
    await waitFor(() => {
      expect(within(main).queryByRole('button', { name: Ar['approvals.gate.approve'] })).toBeInTheDocument();
    });
    expect(within(main).queryByRole('button', { name: Ar['approvals.gate.bypass'] })).not.toBeInTheDocument();
  });

  it('offers no bypass on an environment that does not allow it', async () => {
    mountRun({ bypass: false });
    const main = await findPage(TITLE);
    expect(await within(main).findByRole('button', { name: Ar['approvals.gate.approve'] })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: Ar['approvals.gate.bypass'] })).not.toBeInTheDocument();
    });
  });
});

describe('approving a deploy', () => {
  it('opens a slide, offers no password, and sends the code and the comment', async () => {
    const view = mountRun({ role: 'developer' });
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    expect(view.router.state.location.search).toEqual({ slide: `deployment-approve-${APPROVAL_ID}` });
    expect(await within(dialog).findByLabelText(Ar['factor.method.totp'])).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(Ar['factor.method.password'])).not.toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: Ar['factor.method.password'] })).not.toBeInTheDocument();

    await view.user.type(within(dialog).getByLabelText(Ar['approvals.decide.comment']), 'Checked the migration');
    await enterCode(view, dialog);
    const runsBefore = view.reads.runs;
    const mineBefore = view.state.mineQueries.length;
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Path: `${API}/${view.orgId}/projects/${PROJECT_ID}/runs/${RUN_ID}/approvals/${APPROVAL_ID}/approve`,
      Body: { CurrentFactor: { Method: 'totp', Code: GOOD_CODE }, Comment: 'Checked the migration' },
    });
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(await screen.findByText(arText('approvals.done.approve', { environment: 'production' }))).toBeInTheDocument();
    await waitFor(() => {
      expect(view.reads.runs).toBeGreaterThan(runsBefore);
      expect(view.state.mineQueries.length).toBeGreaterThan(mineBefore);
    });
    expect(await screen.findByText(Ar['approvals.state.approved'])).toBeInTheDocument();
  });

  it('refetches the run and shows the new state after the decision', async () => {
    const view = mountRun({ role: 'developer' });
    view.state.onDecided = () => {
      view.detail.current = detailFixture(runFixture({ State: 'running', FinishedAt: undefined }), [
        gatedJob('', { Gate: undefined, State: 'active', StartedAt: '2026-11-05T09:00:00Z' }),
      ]);
    };
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    await enterCode(view, dialog);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));
    await waitFor(() => {
      expect(document.querySelector('[data-gate="approval"]')).toBeNull();
    });
    expect(within(await findPage(TITLE)).getAllByText(Ar['runs.job.state.active']).length).toBeGreaterThan(0);
  });

});

describe('what the second step and the comment must be', () => {
  it('sends no comment when none is written', async () => {
    const view = mountRun({ role: 'developer' });
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    await enterCode(view, dialog);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toEqual({ CurrentFactor: { Method: 'totp', Code: GOOD_CODE } });
  });

  it('asks for the second step before it sends anything', async () => {
    const view = mountRun({ role: 'developer' });
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    await within(dialog).findByLabelText(Ar['factor.method.totp']);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));
    expect(await within(dialog).findByText(Ar['factor.error.required'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('refuses a comment above 1024 bytes without asking the server', async () => {
    const view = mountRun({ role: 'developer' });
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    await enterCode(view, dialog);
    await view.user.click(within(dialog).getByLabelText(Ar['approvals.decide.comment']));
    await view.user.paste('ع'.repeat(513));
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));
    expect(await within(dialog).findByText(Ar['approvals.error.commentLong'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);

    await view.user.clear(within(dialog).getByLabelText(Ar['approvals.decide.comment']));
    await view.user.click(within(dialog).getByLabelText(Ar['approvals.decide.comment']));
    await view.user.paste('ع'.repeat(512));
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
  });

  it.each([
    ['SELF_REVIEW_NOT_ALLOWED', 'لا يمكنك الموافقة على تنفيذ بدأته.', 403],
    ['REVIEWER_NOT_ELIGIBLE', 'لست مراجعًا على هذه البيئة.', 403],
    ['APPROVAL_NOT_PENDING', 'لم يعد الطلب ينتظر قرارًا.', 409],
    ['REVIEWER_NEEDS_SECOND_FACTOR', 'فعّل خطوتك الثانية أولًا.', 403],
  ])('shows the server message for %s and keeps the slide open', async (code, message, status) => {
    const state = newApprovalsState();
    state.failWrite = backendFailure(code, message, status);
    const view = mountRun({ role: 'developer', state });
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    await view.user.type(within(dialog).getByLabelText(Ar['approvals.decide.comment']), 'kept');
    await enterCode(view, dialog);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));
    expect(await within(dialog).findByText(message)).toBeInTheDocument();
    expect(screen.getByRole('dialog', { name: Ar['approvals.decide.approve.title'] })).toBeInTheDocument();
    expect(within(dialog).getByLabelText(Ar['factor.method.totp'])).toHaveValue('');
    expect(within(dialog).getByLabelText(Ar['approvals.decide.comment'])).toHaveValue('kept');
  });

  it('tells a person without a second step to set one up, and offers no password', async () => {
    const view = mountRun({ role: 'developer', secondFactor: false });
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    expect(await within(dialog).findByText(Ar['approvals.factor.none'])).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(Ar['factor.method.password'])).not.toBeInTheDocument();
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.factor.setup'] }));
    await findPage(Ar['shell.nav.security']);
  });

  it('says the request is no longer waiting when someone else decided it first', async () => {
    const state = newApprovalsState();
    const view = mountRun({ role: 'developer', state });
    const dialog = await openSlide(view, Ar['approvals.gate.approve'], Ar['approvals.decide.approve.title']);
    state.approvals = [approvalFixture({ State: 'rejected' })];
    await view.client.invalidateQueries();
    expect(await within(dialog).findByText(arText('approvals.decide.notPending', { state: Ar['approvals.state.rejected'] }))).toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: Ar['approvals.decide.approve.submit'] })).not.toBeInTheDocument();
  });
});

describe('rejecting a deploy', () => {
  it('sends the reject call with the code and an optional comment', async () => {
    const view = mountRun({ role: 'developer' });
    const dialog = await openSlide(view, Ar['approvals.gate.reject'], Ar['approvals.decide.reject.title']);
    await view.user.type(within(dialog).getByLabelText(Ar['approvals.decide.comment']), 'Wrong build');
    await enterCode(view, dialog);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.reject.submit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Path).toMatch(new RegExp(`/approvals/${APPROVAL_ID}/reject$`));
    expect(view.state.writes[0]?.Body).toEqual({ CurrentFactor: { Method: 'totp', Code: GOOD_CODE }, Comment: 'Wrong build' });
    expect(await screen.findByText(arText('approvals.done.reject', { environment: 'production' }))).toBeInTheDocument();
    expect(await screen.findByText(Ar['approvals.state.rejected'])).toBeInTheDocument();
  });

  it('uses a recovery code when the person picks it', async () => {
    const view = mountRun({ role: 'developer' });
    const dialog = await openSlide(view, Ar['approvals.gate.reject'], Ar['approvals.decide.reject.title']);
    await view.user.click(await within(dialog).findByRole('button', { name: Ar['factor.method.recovery_code'] }));
    await view.user.type(within(dialog).getByLabelText(Ar['factor.method.recovery_code']), 'ABCD-EFGH-JKMN-PQ00');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.reject.submit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toEqual({ CurrentFactor: { Method: 'recovery_code', Code: 'ABCD-EFGH-JKMN-PQ00' } });
  });
});

const twoGates = (): IApprovalsState => {
  const state = newApprovalsState();
  const other = approvalFixture({ Id: OTHER_APPROVAL_ID, EnvironmentId: '0198a000-0000-7000-8000-000000000a02', Environment: 'staging' });
  state.approvals = [approvalFixture(), other];
  state.mine = [approvalFixture(), other];
  return state;
};

const TWO_JOBS = [
  gatedJob('approval'),
  gatedJob('approval', {
    Id: '0198a000-0000-7000-8000-000000000803',
    Key: 'staging',
    JobId: 'staging',
    Name: 'staging',
    Environment: 'staging',
    EnvironmentId: '0198a000-0000-7000-8000-000000000a02',
  }),
];

describe('bypassing the rules', () => {
  it('needs a reason, sends it with the code, and shows the danger action with an icon', async () => {
    const view = mountRun({ role: 'admin' });
    const main = await findPage(TITLE);
    const button = await within(main).findByRole('button', { name: Ar['approvals.gate.bypass'] });
    expect(button).toHaveAttribute('data-variant', 'danger');
    expect(button.querySelector('svg')).not.toBeNull();
    await view.user.click(button);
    const dialog = await screen.findByRole('dialog', { name: Ar['approvals.decide.bypass.title'] });
    expect(view.router.state.location.search).toEqual({ slide: `deployment-bypass-${APPROVAL_ID}` });

    await enterCode(view, dialog);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.bypass.submit'] }));
    expect(await within(dialog).findByText(Ar['approvals.error.reasonEmpty'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);

    await view.user.click(within(dialog).getByLabelText(Ar['approvals.decide.reason']));
    await view.user.paste('ع'.repeat(513));
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.bypass.submit'] }));
    expect(await within(dialog).findByText(Ar['approvals.error.reasonLong'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);

    await view.user.clear(within(dialog).getByLabelText(Ar['approvals.decide.reason']));
    await view.user.type(within(dialog).getByLabelText(Ar['approvals.decide.reason']), 'Security fix for the login page');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.bypass.submit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Path).toMatch(new RegExp(`/approvals/${APPROVAL_ID}/bypass$`));
    expect(view.state.writes[0]?.Body).toEqual({
      CurrentFactor: { Method: 'totp', Code: GOOD_CODE },
      Reason: 'Security fix for the login page',
    });
    expect(await screen.findByText(arText('approvals.done.bypass', { environment: 'production' }))).toBeInTheDocument();
    expect(await screen.findByText(Ar['approvals.state.bypassed'])).toBeInTheDocument();
  });

  it('shows the server message when the environment does not allow it', async () => {
    const state = newApprovalsState();
    state.failWrite = backendFailure('BYPASS_NOT_ALLOWED', 'هذه البيئة لا تسمح بتخطي القواعد.', 403);
    const view = mountRun({ role: 'owner', state });
    const dialog = await openSlide(view, Ar['approvals.gate.bypass'], Ar['approvals.decide.bypass.title']);
    await view.user.type(within(dialog).getByLabelText(Ar['approvals.decide.reason']), 'Urgent');
    await enterCode(view, dialog);
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.bypass.submit'] }));
    expect(await within(dialog).findByText('هذه البيئة لا تسمح بتخطي القواعد.')).toBeInTheDocument();
    expect(within(dialog).getByLabelText(Ar['factor.method.totp'])).toHaveValue('');
  });

  it('starts the next decision with an empty comment, reason and code', async () => {
    const view = mountRun({ role: 'owner', state: twoGates(), jobs: TWO_JOBS });
    const main = await findPage(TITLE);
    const approvals = await within(main).findAllByRole('button', { name: Ar['approvals.gate.approve'] });
    await view.user.click(approvals[0] ?? main);
    const first = await screen.findByRole('dialog', { name: Ar['approvals.decide.approve.title'] });
    await view.user.type(within(first).getByLabelText(Ar['approvals.decide.comment']), 'first comment');
    await enterCode(view, first);
    await view.user.click(within(first).getByRole('button', { name: Ar['approvals.decide.approve.submit'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    const bypasses = await within(main).findAllByRole('button', { name: Ar['approvals.gate.bypass'] });
    await view.user.click(bypasses.at(-1) ?? main);
    const second = await screen.findByRole('dialog', { name: Ar['approvals.decide.bypass.title'] });
    expect(within(second).getByLabelText(Ar['approvals.decide.reason'])).toHaveValue('');
    expect(await within(second).findByLabelText(Ar['factor.method.totp'])).toHaveValue('');
  });
});

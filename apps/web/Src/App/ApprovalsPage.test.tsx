import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import {
  APPROVAL_ID,
  approvalFixture,
  approvalsRoute,
  newApprovalsState,
  OTHER_APPROVAL_ID,
  type IApprovalsState,
} from '@/Core/Testing/ApprovalsBackend';
import { mountRun, RUN_URL } from '@/Core/Testing/ApprovalsHarness';
import { overviewFixture, startBackend } from '@/Core/Testing/Backend';
import { isolate } from '@/I18n/Translate';
import { findPage } from '@/Core/Testing/Pages';
import { PROJECT_ID, RUN_ID } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

const mount = (url: string, role = 'owner', state: IApprovalsState = newApprovalsState(), pending = 0) => {
  const backend = startBackend({ role, routes: [approvalsRoute(state)], overview: overviewFixture({ Approvals: pending }) });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

const menu = () => screen.getByRole('navigation', { name: Ar['shell.nav.label'] });

describe('the page of approvals waiting for you', () => {
  it('lists only the requests the server says you may decide, with Mine=true', async () => {
    const state = newApprovalsState();
    state.mine = [approvalFixture(), approvalFixture({ Id: OTHER_APPROVAL_ID, Environment: 'staging', RunNumber: 9, Ref: 'refs/tags/v1.2.0' })];
    const view = mount('/approvals', 'developer', state);
    const main = await findPage(Ar['approvals.page.title']);
    const list = await within(main).findByRole('list', { name: Ar['approvals.page.list'] });
    const rows = within(list).getAllByRole('link');
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent(arText('approvals.page.row', { environment: 'production', number: 7 }));
    expect(rows[1]).toHaveTextContent(arText('approvals.page.row', { environment: 'staging', number: 9 }));
    expect(rows[1]).toHaveTextContent('v1.2.0');
    expect(state.mineQueries).toEqual(['?Mine=true']);
    expect(view.router.state.location.pathname).toBe('/approvals');
  });

  it('links each request to its run', async () => {
    const view = mount('/approvals');
    const main = await findPage(Ar['approvals.page.title']);
    const link = await within(main).findByRole('link', { name: new RegExp(arText('approvals.page.row', { environment: 'production', number: 7 })) });
    expect(link).toHaveAttribute('href', `/projects/${PROJECT_ID}/runs/${RUN_ID}`);
    await view.user.click(link);
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(RUN_URL);
    });
  });

  it('shows the empty state', async () => {
    const empty = newApprovalsState();
    empty.mine = [];
    mount('/approvals', 'owner', empty);
    expect(await screen.findByText(Ar['approvals.page.empty'])).toBeInTheDocument();
  });

  it('shows the error state with a retry that asks again', async () => {
    const state = newApprovalsState();
    state.failMine = true;
    const view = mount('/approvals', 'owner', state);
    const main = await findPage(Ar['approvals.page.title']);
    expect(await within(main).findByText(Ar['approvals.page.error'])).toBeInTheDocument();
    state.failMine = false;
    await view.user.click(within(main).getByRole('button', { name: Ar['status.retry'] }));
    expect(await within(main).findByRole('list', { name: Ar['approvals.page.list'] })).toBeInTheDocument();
  });

  it('is not an open page for a viewer', async () => {
    const view = mount('/approvals', 'viewer');
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
    expect(view.state.mineQueries).toEqual([]);
  });

  it('shows a loading state while the list arrives', async () => {
    const state = newApprovalsState();
    state.hangMine = true;
    mount('/approvals', 'owner', state);
    const main = await findPage(Ar['approvals.page.title']);
    expect(await within(main).findByRole('status', { name: Ar['approvals.page.loading'] })).toBeInTheDocument();
  });
});

describe('the counter of approvals waiting for you', () => {
  it('shows the count next to the page in the side menu and as a row on the home page', async () => {
    const view = mount('/', 'developer', newApprovalsState(), 3);
    const main = await findPage(Ar['home.title']);
    const counter = await within(menu()).findByLabelText(arText('shell.count.approvals', { count: 3 }));
    expect(counter).toHaveTextContent('3');
    expect(counter.closest('a')).toHaveTextContent(Ar['shell.nav.approvals']);
    const row = await within(main).findByRole('button', { name: new RegExp(Ar['home.attention.approvals.title']) });
    expect(row).toHaveTextContent('3');
    await view.user.click(row);
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe('/approvals');
    });
  });

  it('shows no counter and no row when nothing waits', async () => {
    mount('/', 'developer', newApprovalsState(), 0);
    const main = await findPage(Ar['home.title']);
    expect(await within(main).findByText(Ar['home.attention.calm'])).toBeInTheDocument();
    expect(within(menu()).queryByLabelText(new RegExp(isolate('0')))).not.toBeInTheDocument();
    expect(within(main).queryByText(Ar['home.attention.approvals.title'])).not.toBeInTheDocument();
  });

  it('gives a viewer neither the menu entry nor the counter', async () => {
    mount('/', 'viewer', newApprovalsState(), 0);
    await findPage(Ar['home.title']);
    expect(within(menu()).queryByRole('link', { name: Ar['shell.nav.approvals'] })).not.toBeInTheDocument();
  });

  it('opens the page from the side menu', async () => {
    const view = mount('/', 'maintainer', newApprovalsState(), 0);
    await findPage(Ar['home.title']);
    await view.user.click(within(menu()).getByRole('link', { name: Ar['shell.nav.approvals'] }));
    await findPage(Ar['approvals.page.title']);
    expect(within(menu()).getByRole('link', { name: Ar['shell.nav.approvals'] })).toHaveAttribute('aria-current', 'page');
  });
});

describe('opening a decision slide by its address', () => {
  const OTHER = '0198a000-0000-7000-8000-0000000000aa';

  it.each([
    ['viewer', `deployment-approve-${APPROVAL_ID}`],
    ['viewer', `deployment-reject-${APPROVAL_ID}`],
    ['developer', `deployment-bypass-${APPROVAL_ID}`],
    ['maintainer', `deployment-bypass-${APPROVAL_ID}`],
    ['owner', 'deployment-approve-not-an-id'],
    ['owner', `deployment-approve-${APPROVAL_ID.toUpperCase()}`],
    ['owner', `deployment-other-${APPROVAL_ID}`],
  ])('refuses %s for %s', async (role, slide) => {
    mountRun({ role, url: `${RUN_URL}?slide=${slide}` });
    expect(await screen.findByText(Ar['toast.unknownPanel.title'])).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('opens the slide for a reviewer and shows the request', async () => {
    mountRun({ role: 'developer', url: `${RUN_URL}?slide=deployment-approve-${APPROVAL_ID}` });
    const dialog = await screen.findByRole('dialog', { name: Ar['approvals.decide.approve.title'] });
    expect(await within(dialog).findByLabelText(Ar['factor.method.totp'])).toBeInTheDocument();
    expect(within(dialog).getByText('production')).toBeInTheDocument();
  });

  it('says that an unknown request is not in the run', async () => {
    mountRun({ role: 'developer', url: `${RUN_URL}?slide=deployment-approve-${OTHER}` });
    const dialog = await screen.findByRole('dialog', { name: Ar['approvals.decide.approve.title'] });
    expect(await within(dialog).findByText(Ar['approvals.decide.missing'])).toBeInTheDocument();
  });

  it('asks to open the request from the run when the address is not a run', async () => {
    mount('/approvals?slide=deployment-approve-' + APPROVAL_ID, 'developer');
    const dialog = await screen.findByRole('dialog', { name: Ar['approvals.decide.approve.title'] });
    expect(await within(dialog).findByText(Ar['approvals.decide.noRun'])).toBeInTheDocument();
  });
});

describe('what stays after a decision', () => {
  it('keeps the code and the reason out of the address, the storage, the history and the cache', async () => {
    const reason = 'Reason that must not stay anywhere';
    const SECRET_CODE = '482915';
    const view = mountRun({ role: 'owner' });
    const main = await findPage(arText('runs.row.number', { number: 7 }));
    await view.user.click(await within(main).findByRole('button', { name: Ar['approvals.gate.bypass'] }));
    const dialog = await screen.findByRole('dialog', { name: Ar['approvals.decide.bypass.title'] });
    await view.user.type(within(dialog).getByLabelText(Ar['approvals.decide.reason']), reason);
    await view.user.type(await within(dialog).findByLabelText(Ar['factor.method.totp']), SECRET_CODE);
    expect(view.router.state.location.href).not.toContain(SECRET_CODE);
    expect(view.router.state.location.href).not.toContain('Reason');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['approvals.decide.bypass.submit'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toMatchObject({ CurrentFactor: { Code: SECRET_CODE } });
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    const stored = JSON.stringify([
      Object.entries(window.localStorage),
      Object.entries(window.sessionStorage),
      window.history.state,
      view.router.state.location.href,
      view.router.state.location.search,
    ]);
    expect(stored).not.toContain(SECRET_CODE);
    expect(stored).not.toContain(reason);
    const cached = JSON.stringify(view.client.getQueryCache().getAll().map((query) => [query.queryKey, query.state.data]));
    expect(cached).not.toContain(SECRET_CODE);
    expect(view.client.getMutationCache().getAll()).toEqual([]);
    expect(document.body.textContent).not.toContain(SECRET_CODE);
    expect(document.querySelectorAll('input, textarea')).toHaveLength(0);
  });
});

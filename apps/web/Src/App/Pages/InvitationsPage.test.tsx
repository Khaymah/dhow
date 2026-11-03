import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope, ORG_ID } from '@/Core/Testing/Fetch';
import { gate } from '@/Core/Testing/Gate';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

configure({ asyncUtilTimeout: 5000 });

const PATH = `/api/v1/organizations/${ORG_ID}/invitations`;

const invitation = (Id: string, Email: string, Expired: boolean) => ({
  Id,
  Email,
  Role: 'developer',
  InvitedByName: 'صبري داود',
  CreatedAt: '2026-10-20T08:00:00Z',
  ExpiresAt: Expired ? '2026-10-27T08:00:00Z' : '2026-11-05T08:00:00Z',
  SentAt: '2026-10-20T08:00:00Z',
  SendCount: 1,
  EmailStatus: 'disabled',
  Expired,
});

const listed =
  (items: unknown[]): TRouteHandler =>
  (request, url) =>
    request.method === 'GET' && url.pathname === PATH ? envelope({ Success: true, Data: items }) : undefined;

const open = async (routes: TRouteHandler[], role = 'owner') => {
  startBackend({ role, routes });
  const router = createAppRouter(createMemoryHistory({ initialEntries: ['/org/invitations'] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const page = await findPage(Ar['shell.nav.invitations']);
  return { router, user, page };
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the invitations page', () => {
  it('lists pending invitations before expired ones, each with its state and actions', async () => {
    const { page } = await open([
      listed([invitation('0198a000-0000-7000-8000-0000000000e2', 'old@future.dev', true), invitation('0198a000-0000-7000-8000-0000000000e1', 'new@future.dev', false)]),
    ]);
    const list = await within(page).findByRole('list', { name: Ar['invitations.title'] });
    const rows = within(list).getAllByRole('listitem');
    expect(rows[0]).toHaveTextContent('new@future.dev');
    expect(rows[0]).toHaveTextContent(Ar['invitations.status.pending']);
    expect(rows[1]).toHaveTextContent('old@future.dev');
    expect(rows[1]).toHaveTextContent(Ar['invitations.status.expired']);
    for (const email of ['new@future.dev', 'old@future.dev']) {
      expect(within(list).getByRole('button', { name: Ar['invitations.resendLabel'].replace('{email}', isolate(email)) })).toBeInTheDocument();
      expect(within(list).getByRole('button', { name: Ar['invitations.revokeLabel'].replace('{email}', isolate(email)) })).toBeInTheDocument();
    }
    expect(within(page).getByText(Ar['invitations.counts'].replace('{pending}', isolate('1')).replace('{expired}', isolate('1')))).toBeInTheDocument();
  });

  it('isolates the email and the inviter name', async () => {
    const { page } = await open([listed([invitation('0198a000-0000-7000-8000-0000000000e1', 'new@future.dev', false)])]);
    const list = await within(page).findByRole('list', { name: Ar['invitations.title'] });
    expect(within(list).getByText('new@future.dev').tagName).toBe('BDI');
    expect(within(list).getByText('صبري داود').tagName).toBe('BDI');
  });

  it('makes the invite button the one primary action, and opens the invite slide', async () => {
    const { page, user, router } = await open([listed([])]);
    expect(await within(page).findByText(Ar['invitations.empty'])).toBeInTheDocument();
    const primary = page.querySelectorAll('[data-variant="primary"]');
    expect(primary).toHaveLength(1);
    expect(primary[0]).toHaveTextContent(Ar['organization.invite']);
    await user.click(within(page).getByRole('button', { name: Ar['organization.invite'] }));
    expect(await screen.findByRole('dialog', { name: Ar['invite.title'] })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ slide: 'organization-invite' });
  });

  it('shows a skeleton while the invitations load', async () => {
    const hold = gate(/\/invitations$/, 'hang');
    const { page } = await open([hold.route]);
    expect(await within(page).findByLabelText(Ar['invitations.loading'])).toBeInTheDocument();
    expect(hold.hits()).toBe(1);
  });

  it('shows an error with a retry when the invitations cannot be loaded', async () => {
    const failing: TRouteHandler = (request, url) =>
      request.method === 'GET' && url.pathname === PATH ? backendFailure('INTERNAL', 'x', 500) : undefined;
    const { page } = await open([failing]);
    expect(await within(page).findByText(Ar['invitations.error'])).toBeInTheDocument();
    expect(within(page).getByRole('button', { name: Ar['status.retry'] })).toBeInTheDocument();
  });
});

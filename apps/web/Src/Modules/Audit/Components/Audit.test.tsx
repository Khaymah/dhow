import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { usePaletteStore } from '@/Core/Store/usePaletteStore';
import { auditFixture, backendFailure, membersFixture, startBackend, type IBackendOptions } from '@/Core/Testing/Backend';
import { envelope, USER_ID } from '@/Core/Testing/Fetch';
import { findPage } from '@/Core/Testing/Pages';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const mount = async (options: IBackendOptions = {}, url = '/audit') => {
  const backend = startBackend({ audit: auditFixture(45), ...options });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = await findPage(Ar['audit.title']);
  return { ...backend, router, user, dialog };
};

const verifyButton = () => screen.findByRole('button', { name: Ar['audit.verify.action'] });

describe('the audit log page', () => {
  it('lists events with what happened, who did it, from which address and when', async () => {
    const { dialog } = await mount();
    const list = await within(dialog).findByRole('list', { name: Ar['audit.list.title'] });
    const rows = within(list).getAllByRole('listitem');
    expect(rows).toHaveLength(20);
    const first = rows[0];
    expect(first).toHaveTextContent(Ar['audit.action.auth.login']);
    expect(first).toHaveTextContent('Sabry Dawood');
    expect(first).toHaveTextContent('203.0.113.1');
    expect(first).toHaveTextContent(`منذ ${isolate('10')} دقائق`);
    expect(rows[2]).toHaveTextContent(Ar['audit.action.auth.login_failed']);
    expect(rows[2]).toHaveTextContent(Ar['audit.actor.visitor']);
    expect(within(dialog).getByText(arText('audit.list.total', { count: 45 }))).toBeInTheDocument();
  });

  it('asks the API for one page at a time and moves between pages', async () => {
    const { dialog, user, calls } = await mount();
    await within(dialog).findByRole('list', { name: Ar['audit.list.title'] });
    expect(within(dialog).getByText(Ar['pager.status'].replace('{page}', isolate('1')).replace('{pages}', isolate('3')))).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: Ar['pager.next'] }));
    await waitFor(() => {
      expect(within(dialog).getByText(Ar['pager.status'].replace('{page}', isolate('2')).replace('{pages}', isolate('3')))).toBeInTheDocument();
    });
    const urls = calls.map((call) => new URL(call.url)).filter((url) => url.pathname.endsWith('/audit-events'));
    expect(urls.map((url) => [url.searchParams.get('Page'), url.searchParams.get('Limit')])).toEqual([
      ['1', '20'],
      ['2', '20'],
    ]);
    await user.click(within(dialog).getByRole('button', { name: Ar['pager.next'] }));
    await waitFor(() => {
      expect(within(dialog).getByText(Ar['pager.status'].replace('{page}', isolate('3')).replace('{pages}', isolate('3')))).toBeInTheDocument();
    });
    expect(within(dialog).getByRole('button', { name: Ar['pager.next'] })).toBeDisabled();
    expect(within(await within(dialog).findByRole('list', { name: Ar['audit.list.title'] })).getAllByRole('listitem')).toHaveLength(5);
  });

  it('shows an empty log in words', async () => {
    await mount({ audit: [] });
    expect(await screen.findByText(Ar['audit.empty'])).toBeInTheDocument();
  });

  it('names an action it does not know by its area in words, and keeps the raw name beside it', async () => {
    const [event] = auditFixture(1);
    if (!event) throw new Error('the audit fixture is missing');
    const { dialog } = await mount({ audit: ['billing.invoice.paid', 'runner.labels_changed'].map((Action) => ({ ...event, Action })) });
    const rows = within(await within(dialog).findByRole('list', { name: Ar['audit.list.title'] })).getAllByRole('listitem');
    expect(rows[0]).toHaveTextContent(Ar['audit.group.other']);
    expect(within(rows[0] ?? dialog).getByText('billing.invoice.paid')).toBeInTheDocument();
    expect(rows[1]).toHaveTextContent(Ar['audit.group.runner']);
    expect(within(rows[1] ?? dialog).getByText('runner.labels_changed')).toBeInTheDocument();
  });

  it('names runner and job events in words, not raw keys', async () => {
    const [event] = auditFixture(1);
    if (!event) throw new Error('the audit fixture is missing');
    const actions = ['job.created', 'runner.registered', 'runner.registration_token_created'];
    const { dialog } = await mount({ audit: actions.map((Action) => ({ ...event, Action })) });
    const list = await within(dialog).findByRole('list', { name: Ar['audit.list.title'] });
    expect(list).toHaveTextContent(Ar['audit.action.job.created']);
    expect(list).toHaveTextContent(Ar['audit.action.runner.registered']);
    expect(list).toHaveTextContent(Ar['audit.action.runner.registration_token_created']);
    expect(within(list).queryByText('job.created')).not.toBeInTheDocument();
  });

  it('shows an error with a retry when the log cannot be loaded', async () => {
    let failing = true;
    const { user } = await mount({
      routes: [
        (_request, url) =>
          failing && url.pathname.endsWith('/audit-events') ? backendFailure('INTERNAL', 'حدث خطأ في السيرفر.', 500) : undefined,
      ],
    });
    expect(await screen.findByText(Ar['audit.error.title'])).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole('button', { name: Ar['status.retry'] }));
    expect(await screen.findByRole('list', { name: Ar['audit.list.title'] })).toBeInTheDocument();
  });
});

describe('verifying the chain', () => {
  it('says the chain was not checked yet, then that it is intact with the number of events', async () => {
    const { user, calls } = await mount();
    expect(await screen.findByText(Ar['audit.verify.idle.title'])).toBeInTheDocument();
    await user.click(await verifyButton());
    expect(await screen.findByText(Ar['audit.verify.intact.title'])).toBeInTheDocument();
    expect(screen.getByText(arText('audit.verify.intact.sub', { count: 45 }))).toBeInTheDocument();
    const verify = calls.find((call) => call.url.endsWith('/audit-events/verify'));
    expect(verify?.method).toBe('GET');
    expect(screen.queryByText(Ar['audit.verify.idle.title'])).not.toBeInTheDocument();
  });

  it('says the chain is broken and where it broke', async () => {
    const { user } = await mount({
      verify: () => envelope({ Success: true, Data: { Valid: false, Checked: 12, FirstBrokenId: '0198a000-0000-7000-8000-000000000013' } }),
    });
    await user.click(await verifyButton());
    expect(await screen.findByText(Ar['audit.verify.broken.title'])).toBeInTheDocument();
    expect(screen.getByText(new RegExp(`فحصنا ${isolate('12')} حدث`))).toBeInTheDocument();
    expect(screen.getByText('0198a000-0000-7000-8000-000000000013')).toBeInTheDocument();
    expect(screen.queryByText(Ar['audit.verify.intact.title'])).not.toBeInTheDocument();
  });

  it('shows a failed check and lets the person retry it', async () => {
    let failing = true;
    const { user } = await mount({
      verify: () => (failing ? backendFailure('INTERNAL', 'حدث خطأ في السيرفر.', 500) : envelope({ Success: true, Data: { Valid: true, Checked: 45 } })),
    });
    await user.click(await verifyButton());
    expect(await screen.findByText(Ar['audit.verify.error'])).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole('button', { name: Ar['status.retry'] }));
    expect(await screen.findByText(Ar['audit.verify.intact.title'])).toBeInTheDocument();
  });

  it('forgets an old result when the person leaves the page and comes back', async () => {
    const { user, router } = await mount();
    await user.click(await verifyButton());
    await screen.findByText(Ar['audit.verify.intact.title']);
    const nav = screen.getByRole('navigation', { name: Ar['shell.nav.label'] });
    await user.click(within(nav).getByRole('link', { name: Ar['shell.nav.home'] }));
    await waitFor(() => {
      expect(router.state.location.href).toBe('/');
    });
    await screen.findByRole('heading', { level: 1, name: Ar['home.title'] });
    await user.click(within(nav).getByRole('link', { name: Ar['shell.nav.audit'] }));
    expect(await screen.findByText(Ar['audit.verify.idle.title'])).toBeInTheDocument();
  });
});

describe('who can open the audit log', () => {
  it.each(['owner', 'admin'])('shows the menu entry to the %s and opens the page from it', async (role) => {
    const backend = startBackend({ role, audit: auditFixture(3) });
    const router = createAppRouter(createMemoryHistory({ initialEntries: ['/'] }));
    const user = userEvent.setup();
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    expect(backend.state.role).toBe(role);
    await user.click(await screen.findByRole('link', { name: Ar['shell.nav.audit'] }));
    expect(await findPage(Ar['audit.title'])).toBeInTheDocument();
    expect(router.state.location.href).toBe('/audit');
  });

  it.each(['maintainer', 'developer', 'viewer'])('hides the menu entry from the %s', async (role) => {
    startBackend({ role });
    const router = createAppRouter(createMemoryHistory({ initialEntries: ['/'] }));
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    await screen.findByRole('navigation', { name: Ar['shell.nav.label'] });
    expect(screen.queryByRole('link', { name: Ar['shell.nav.audit'] })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['shell.nav.audit'] })).not.toBeInTheDocument();
  });

  it('shows the not-found state, and no request, when a developer opens the link', async () => {
    const { calls } = startBackend({ role: 'developer', audit: auditFixture(3) });
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: ['/audit'] }))} queryClient={createQueryClient({ retry: false })} />);
    expect(await screen.findByText(Ar['notFound.text'], {}, { timeout: 5000 })).toBeInTheDocument();
    expect(calls.some((call) => call.url.includes('/audit-events'))).toBe(false);
    expect(screen.queryByRole('button', { name: Ar['audit.verify.action'] })).not.toBeInTheDocument();
  });

  it('shows the same forbidden state when the server answers 403 anyway', async () => {
    await mount({
      routes: [(_request, url) => (url.pathname.endsWith('/audit-events') ? backendFailure('FORBIDDEN', 'غير مسموح.', 403) : undefined)],
    });
    expect(await screen.findByText(Ar['audit.forbidden'])).toBeInTheDocument();
  });

  it('hides the command from the palette for a viewer', async () => {
    startBackend({ role: 'viewer' });
    const user = userEvent.setup();
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: ['/'] }))} queryClient={createQueryClient({ retry: false })} />);
    await findPage(Ar['home.title']);
    await user.keyboard('{Control>}k{/Control}');
    await user.type(await screen.findByRole('combobox'), Ar['palette.command.audit']);
    expect(screen.queryByRole('option', { name: new RegExp(Ar['palette.command.audit']) })).not.toBeInTheDocument();
  });
});

describe('names and addresses in the audit log', () => {
  beforeEach(() => {
    usePaletteStore.setState({ Open: false });
  });

  it('isolates the actor name and the address so an Arabic name keeps its place in English text', async () => {
    const members = membersFixture().map((member) => (member.UserId === USER_ID ? { ...member, Name: 'صبري داود' } : member));
    const { dialog } = await mount({ members, audit: auditFixture(3) });
    const list = await within(dialog).findByRole('list', { name: Ar['audit.list.title'] }, { timeout: 5000 });
    const first = within(list).getAllByRole('listitem')[0];
    if (first === undefined) throw new Error('no audit row');
    expect(within(first).getByText('صبري داود').tagName).toBe('BDI');
    expect(within(first).getByText('203.0.113.1').tagName).toBe('BDI');
    expect(within(first).getByText('صبري داود').nextSibling?.textContent).toBe(' · ');
  });

  it('isolates the actor, the address and the ids on the event page', async () => {
    const members = membersFixture().map((member) => (member.UserId === USER_ID ? { ...member, Name: 'صبري داود' } : member));
    const events = auditFixture(1);
    const event = events[0];
    if (event === undefined) throw new Error('no audit event');
    startBackend({
      members,
      audit: events,
      routes: [
        (request, url) =>
          request.method === 'GET' && url.pathname.endsWith(`/audit-events/${event.Id}`) ? envelope({ Success: true, Data: event }) : undefined,
      ],
    });
    const router = createAppRouter(createMemoryHistory({ initialEntries: [`/audit/${event.Id}`] }));
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    const page = await findPage(Ar['audit.detail.title']);
    expect((await within(page).findByText('صبري داود', {}, { timeout: 5000 })).tagName).toBe('BDI');
    expect(within(page).getByText(event.Ip).tagName).toBe('BDI');
    expect(within(page).getByText(event.Id).tagName).toBe('BDI');
    expect(within(page).getByText(event.TargetId).tagName).toBe('BDI');
  });
});

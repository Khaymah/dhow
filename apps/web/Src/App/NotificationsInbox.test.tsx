import { configure, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { mount } from '@/App/NotificationsHarness';
import { inboxFixture, newNotificationsState } from '@/Core/Testing/NotificationsBackend';
import { findPage } from '@/Core/Testing/Pages';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const INBOX_URL = '/notifications';
const RUN_LINK = `/projects/${PROJECT_ID}/runs/12`;

describe('the inbox page', () => {
  it('writes every notification from its key in the UI language, with the plain values', async () => {
    mount(INBOX_URL);
    const main = await findPage(Ar['notifications.title']);
    const list = await within(main).findByRole('list', { name: Ar['notifications.title'] });
    const rows = within(list).getAllByRole('listitem');
    expect(rows).toHaveLength(2);
    const [failed, run] = rows;
    if (failed === undefined || run === undefined) throw new Error('missing rows');
    expect(within(failed).getByRole('link', { name: Ar['notifications.inbox.deployment.finished.failed'] })).toHaveAttribute('href', RUN_LINK);
    expect(failed).toHaveTextContent('Web app');
    expect(failed).toHaveTextContent('production');
    expect(failed).toHaveTextContent(Ar['notifications.inbox.unread']);
    expect(run).toHaveTextContent(Ar['notifications.inbox.run.finished']);
    expect(within(run).queryByRole('link')).not.toBeInTheDocument();
    expect(run).not.toHaveTextContent(Ar['notifications.inbox.unread']);
  });

  it('shows the unread count in the shell and lowers it when a notification is read', async () => {
    const view = mount(INBOX_URL);
    const main = await findPage(Ar['notifications.title']);
    const bell = await screen.findByRole('button', { name: `${Ar['shell.notifications.unread']} 1` });
    expect(bell).not.toHaveAttribute('aria-label');

    await view.user.click(await within(main).findByRole('button', { name: /مقروء: / }));
    await waitFor(() => {
      expect(screen.getByRole('button', { name: Ar['shell.notifications'] })).toBeInTheDocument();
    });
    expect(view.state.writes).toHaveLength(1);
    expect(view.state.writes[0]?.Path).toMatch(/\/me\/notifications\/.+\/read$/);
  });

  it('marks everything as read and filters the unread ones', async () => {
    const view = mount(INBOX_URL);
    const main = await findPage(Ar['notifications.title']);
    await view.user.click(await within(main).findByRole('button', { name: Ar['notifications.inbox.filter.unread'] }));
    const list = await within(main).findByRole('list', { name: Ar['notifications.title'] });
    await waitFor(() => {
      expect(within(list).getAllByRole('listitem')).toHaveLength(1);
    });

    await view.user.click(within(main).getByRole('button', { name: Ar['notifications.inbox.markAll'] }));
    await waitFor(() => {
      expect(view.state.inbox.every((item) => item.Read === true)).toBe(true);
    });
    expect(await within(main).findByText(Ar['notifications.inbox.emptyUnread'])).toBeInTheDocument();
  });

  it('opens the link of a notification inside the app and marks it as read', async () => {
    const view = mount(INBOX_URL);
    const main = await findPage(Ar['notifications.title']);
    await view.user.click(await within(main).findByRole('link', { name: Ar['notifications.inbox.deployment.finished.failed'] }));
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(RUN_LINK);
    });
    expect(view.state.writes.some((write) => write.Path.endsWith('/read'))).toBe(true);
  });

  it('shows an empty page when the inbox has nothing', async () => {
    const state = newNotificationsState();
    state.inbox = [];
    mount(INBOX_URL, 'owner', state);
    const main = await findPage(Ar['notifications.title']);
    expect(await within(main).findByText(Ar['notifications.empty'])).toBeInTheDocument();
  });
});

describe('hostile inbox content', () => {
  it('renders markup in the values as plain text and never builds HTML from a server string', async () => {
    const state = newNotificationsState();
    state.inbox = [
      inboxFixture({
        TitleKey: 'inbox.deployment.finished.failed',
        Params: { Project: '<img src=x onerror=alert(1)>', Ref: '<script>alert(1)</script>', Environment: '<b>prod</b>' },
        Link: undefined,
      }),
    ];
    mount(INBOX_URL, 'owner', state);
    const main = await findPage(Ar['notifications.title']);
    const row = (await within(main).findByText(Ar['notifications.inbox.deployment.finished.failed'])).closest('li');
    if (row === null) throw new Error('no row');
    expect(row).toHaveTextContent('<img src=x onerror=alert(1)>');
    expect(row).toHaveTextContent('<script>alert(1)</script>');
    expect(row.querySelector('img, script, b')).toBeNull();
  });

  it('never shows a title key the UI does not know, and never raw server text', async () => {
    const state = newNotificationsState();
    state.inbox = [inboxFixture({ TitleKey: '<b>raw server text</b>', Link: undefined })];
    mount(INBOX_URL, 'owner', state);
    const main = await findPage(Ar['notifications.title']);
    expect(await within(main).findByText(Ar['notifications.inbox.unknown'])).toBeInTheDocument();
    expect(main).not.toHaveTextContent('raw server text');
  });

  it.each(['javascript:alert(1)', '//evil.example/steal', 'https://evil.example/phish', '/ok"onmouseover="x', ' /with space'])(
    'does not turn the link %s into an anchor',
    async (link) => {
      const state = newNotificationsState();
      state.inbox = [inboxFixture({ Link: link })];
      mount(INBOX_URL, 'owner', state);
      const main = await findPage(Ar['notifications.title']);
      expect(await within(main).findByText(Ar['notifications.inbox.deployment.finished.failed'])).toBeInTheDocument();
      const list = within(main).getByRole('list', { name: Ar['notifications.title'] });
      expect(within(list).queryByRole('link')).not.toBeInTheDocument();
    },
  );

  it('keeps unrelated parameters out of the page', async () => {
    const state = newNotificationsState();
    const stray = 'stray-value-9f3a2c';
    state.inbox = [inboxFixture({ Params: { Project: 'Web app', Token: stray, Network: '203.0.113.7' }, Link: undefined })];
    mount(INBOX_URL, 'owner', state);
    const main = await findPage(Ar['notifications.title']);
    expect(await within(main).findByText(Ar['notifications.inbox.deployment.finished.failed'])).toBeInTheDocument();
    expect(main).not.toHaveTextContent(stray);
    expect(main).not.toHaveTextContent('203.0.113.7');
  });
});

describe('the second-step and role changes in the inbox', () => {
  it('writes the change of a second step in words', async () => {
    const state = newNotificationsState();
    state.inbox = [
      inboxFixture({
        Event: 'account.second_factor_changed',
        TitleKey: 'inbox.account.second_factor_changed.self',
        Params: { Change: 'passkey_added' },
        Link: '/?stack=account',
      }),
    ];
    mount(INBOX_URL, 'owner', state);
    const main = await findPage(Ar['notifications.title']);
    expect(await within(main).findByText(Ar['notifications.inbox.account.second_factor_changed.self'])).toBeInTheDocument();
    expect(main).toHaveTextContent(Ar['notifications.inbox.change.passkey_added']);
  });
});

describe('the preferences page', () => {
  it('turns the mute of channel emails on and off', async () => {
    const view = mount('/account/preferences');
    const main = await findPage(Ar['shell.nav.preferences']);
    const box = await within(main).findByRole('checkbox', { name: new RegExp(Ar['notifications.mute.option']) });
    expect(box).toHaveAttribute('aria-checked', 'false');
    await view.user.click(box);
    await waitFor(() => {
      expect(view.state.muted).toBe(true);
    });
    await waitFor(() => {
      expect(within(main).getByRole('checkbox', { name: new RegExp(Ar['notifications.mute.option']) })).toHaveAttribute('aria-checked', 'true');
    });
  });
});

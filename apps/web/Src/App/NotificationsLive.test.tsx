import { configure, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';

import { mount } from '@/App/NotificationsHarness';
import { FakeEventSource, installFakeEventSource } from '@/Core/Testing/FakeEventSource';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const stream = (): FakeEventSource => {
  const found = [...FakeEventSource.instances].reverse().find((source) => source.url.endsWith('/events'));
  if (found === undefined) throw new Error('the shell opened no organization stream');
  return found;
};

describe('the live bell', () => {
  beforeEach(installFakeEventSource);

  it('refreshes the unread count when the organization stream says the inbox changed', async () => {
    const view = mount('/notifications');
    await findPage(Ar['notifications.title']);
    await screen.findByRole('button', { name: `${Ar['shell.notifications.unread']} 1` });
    stream().open();

    view.state.inbox = view.state.inbox.map((item) => ({ ...item, Read: true }));
    stream().emit('change', { Seq: 1, Type: 'inbox.changed', At: '2026-11-08T10:00:00Z' }, '1-1');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: Ar['shell.notifications'] })).toBeInTheDocument();
    });
  });

  it('does not refresh the count for events that are not about the inbox', async () => {
    const view = mount('/notifications');
    await findPage(Ar['notifications.title']);
    await screen.findByRole('button', { name: `${Ar['shell.notifications.unread']} 1` });
    stream().open();

    view.state.inbox = view.state.inbox.map((item) => ({ ...item, Read: true }));
    stream().emit('change', { Seq: 1, Type: 'overview.changed', At: '2026-11-08T10:00:00Z' }, '1-1');
    await new Promise((resolve) => setTimeout(resolve, 600));

    expect(screen.getByRole('button', { name: `${Ar['shell.notifications.unread']} 1` })).toBeInTheDocument();
  });
});

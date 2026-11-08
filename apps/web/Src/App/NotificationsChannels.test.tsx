import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { CHANNEL_ID, DELIVERY_ID, newNotificationsState, OTHER_CHANNEL_ID } from '@/Core/Testing/NotificationsBackend';
import { API, channelRow, mount, named, PAGE_URL, PASSWORD, PROJECT_NAME } from '@/App/NotificationsHarness';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the channels page', () => {
  it('lists the channels with their kind, host and state, and the delivery log for a manager', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    const hook = await channelRow(main, 'ops-hook');
    expect(within(hook).getByText(Ar['notifications.state.active'])).toBeInTheDocument();
    expect(hook).toHaveTextContent('hooks.example.com');
    expect(hook).toHaveTextContent(Ar['notifications.kind.webhook']);
    expect(await within(main).findByText('team-mail')).toBeInTheDocument();
    expect(await within(main).findByText(Ar['notifications.deliveries.title'], { selector: 'h3' })).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('marks a stopped channel and offers to turn it on again with the second step', async () => {
    const state = newNotificationsState();
    state.channels = [{ ...state.channels[0], Enabled: false, DisabledReason: 'failures' }];
    const view = mount(PAGE_URL, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    const hook = await channelRow(main, 'ops-hook');
    expect(within(hook).getByText(Ar['notifications.state.failures'])).toBeInTheDocument();
    expect(within(hook).getByRole('button', { name: named(Ar['notifications.rotate.aria'], 'ops-hook') })).toBeInTheDocument();

    await view.user.click(within(hook).getByRole('button', { name: named(Ar['notifications.enable.aria'], 'ops-hook') }));
    await view.user.click(await within(hook).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(hook).getByRole('button', { name: Ar['notifications.enable.confirm'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toMatchObject({ Method: 'PUT', Body: { Enable: true, Version: 1, CurrentFactor: { Method: 'password' } } });
    expect(await within(main).findByText(Ar['notifications.state.active'])).toBeInTheDocument();
  });

  it('removes a channel only after the second step', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    const hook = await channelRow(main, 'ops-hook');
    await view.user.click(within(hook).getByRole('button', { name: named(Ar['notifications.remove.aria'], 'ops-hook') }));
    await view.user.click(await within(hook).findByRole('button', { name: Ar['notifications.remove.confirm'] }));
    expect(await within(hook).findByText(Ar['factor.error.required'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);

    await view.user.click(within(hook).getByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(hook).getByRole('button', { name: Ar['notifications.remove.confirm'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/notification-channels/${CHANNEL_ID}/remove`,
      Body: { CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    const list = await within(main).findByRole('list', { name: Ar['notifications.channels.title'] });
    await waitFor(() => {
      expect(within(list).queryByText('ops-hook')).not.toBeInTheDocument();
    });
  });
});

describe('the rules of a channel', () => {
  it('sends the chosen events, statuses, environments and roles of an email channel', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    const mail = await channelRow(main, 'team-mail');
    await view.user.click(within(mail).getByRole('button', { name: named(Ar['notifications.rules.open'], 'team-mail') }));
    const form = await within(mail).findByRole('form', { name: named(Ar['notifications.rules.title'], 'team-mail') });

    await view.user.click(within(form).getByRole('checkbox', { name: Ar['notifications.event.run.finished'] }));
    await view.user.click(within(form).getByRole('checkbox', { name: Ar['notifications.status.rolled_back'] }));
    await view.user.click(within(form).getByRole('checkbox', { name: Ar['role.developer'] }));
    await view.user.type(within(form).getByLabelText(Ar['notifications.rules.environments']), 'production, staging');
    await view.user.click(within(form).getByRole('button', { name: Ar['notifications.rules.save'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'PUT',
      Path: `${API}/notification-rules`,
      Body: {
        Rules: [
          {
            ChannelId: OTHER_CHANNEL_ID,
            Events: ['deployment.finished', 'run.finished'],
            Statuses: ['failed', 'rolled_back'],
            Environments: ['production', 'staging'],
            Roles: ['owner', 'admin', 'developer'],
          },
        ],
      },
    });
  });

  it('refuses an empty choice and a bad environment name before it asks the server', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    const hook = await channelRow(main, 'ops-hook');
    await view.user.click(within(hook).getByRole('button', { name: named(Ar['notifications.rules.open'], 'ops-hook') }));
    const form = await within(hook).findByRole('form', { name: named(Ar['notifications.rules.title'], 'ops-hook') });
    expect(within(form).queryByRole('checkbox', { name: Ar['role.owner'] })).not.toBeInTheDocument();

    await view.user.click(within(form).getByRole('checkbox', { name: Ar['notifications.status.failed'] }));
    await view.user.click(within(form).getByRole('button', { name: Ar['notifications.rules.save'] }));
    expect(await within(form).findByText(Ar['notifications.rules.statusesEmpty'])).toBeInTheDocument();

    await view.user.click(within(form).getByRole('checkbox', { name: Ar['notifications.status.failed'] }));
    await view.user.type(within(form).getByLabelText(Ar['notifications.rules.environments']), 'Prod Env');
    await view.user.click(within(form).getByRole('button', { name: Ar['notifications.rules.save'] }));
    expect(await within(form).findByText(Ar['notifications.rules.environmentsInvalid'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });
});

describe('the test message', () => {
  it('queues a test message and points to the delivery log', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    const hook = await channelRow(main, 'ops-hook');
    await view.user.click(within(hook).getByRole('button', { name: Ar['notifications.test.send'] }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toMatchObject({ Method: 'POST', Path: `${API}/notification-channels/${CHANNEL_ID}/test` });
    expect(await screen.findByText(named(Ar['notifications.test.queued'], 'ops-hook'))).toBeInTheDocument();
  });

  it('shows the rate limit and waits before it lets the person try again', async () => {
    const state = newNotificationsState();
    state.rateLimited = true;
    const view = mount(PAGE_URL, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    const hook = await channelRow(main, 'ops-hook');
    await view.user.click(within(hook).getByRole('button', { name: Ar['notifications.test.send'] }));

    expect(await within(hook).findByText(/أرسلت رسائل اختبار كثيرة/)).toBeInTheDocument();
    const wait = within(hook).getAllByRole('button').find((button) => button.textContent.includes('1:00'));
    expect(wait).toBeDisabled();
  });
});

describe('the delivery log', () => {
  it('shows the state, the attempts and the last code, and retries only a failed delivery', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    const log = await within(main).findByRole('list', { name: Ar['notifications.deliveries.title'] });
    const rows = await within(log).findAllByRole('listitem');
    expect(rows).toHaveLength(2);
    const [dead, sent] = rows;
    if (dead === undefined || sent === undefined) throw new Error('missing rows');
    expect(dead).toHaveTextContent(Ar['notifications.deliveries.state.dead']);
    expect(dead).toHaveTextContent(Ar['notifications.code.http_status']);
    expect(dead).toHaveTextContent('502');
    expect(within(sent).queryByRole('button')).not.toBeInTheDocument();

    await view.user.click(within(dead).getByRole('button', { name: named(Ar['notifications.deliveries.retry.aria'], 'ops-hook') }));
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toMatchObject({ Method: 'POST', Path: `${API}/notification-deliveries/${DELIVERY_ID}/retry` });
    await waitFor(() => {
      expect(within(main).queryByRole('button', { name: named(Ar['notifications.deliveries.retry.aria'], 'ops-hook') })).not.toBeInTheDocument();
    });
  });

  it('filters by state', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    await view.user.selectOptions(await within(main).findByLabelText(Ar['notifications.deliveries.filter']), 'sent');
    const log = await within(main).findByRole('list', { name: Ar['notifications.deliveries.title'] });
    await waitFor(() => {
      expect(within(log).getAllByRole('listitem')).toHaveLength(1);
    });
    expect(log).toHaveTextContent(Ar['notifications.deliveries.state.sent']);
  });
});

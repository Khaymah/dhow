import { configure, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { channelRow, mount, PAGE_URL, PROJECT_NAME } from '@/App/NotificationsHarness';
import { findPage } from '@/Core/Testing/Pages';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

describe.each(['developer', 'viewer'])('a %s on the notifications page', (role) => {
  it('reads the channels but has no button that creates, tests, removes or changes one', async () => {
    const view = mount(PAGE_URL, role);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText('ops-hook')).toBeInTheDocument();
    expect(within(main).getByText(Ar['notifications.readOnly'])).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['notifications.channels.new'] })).not.toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['notifications.test.send'] })).not.toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: new RegExp(Ar['notifications.remove.button']) })).not.toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: new RegExp(Ar['notifications.rotate.button']) })).not.toBeInTheDocument();
    expect(within(main).queryByText(Ar['notifications.deliveries.title'], { selector: 'h3' })).not.toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('sees the rules read only', async () => {
    const view = mount(PAGE_URL, role);
    const main = await findPage(PROJECT_NAME);
    const row = await channelRow(main, 'ops-hook');
    await view.user.click(within(row).getByRole('button', { name: /قواعد القناة/ }));
    const form = await within(row).findByRole('form');
    expect(within(form).getByText(Ar['notifications.rules.readOnly'])).toBeInTheDocument();
    expect(within(form).queryByRole('button', { name: Ar['notifications.rules.save'] })).not.toBeInTheDocument();
    for (const box of within(form).getAllByRole('checkbox')) expect(box).toBeDisabled();
  });

  it('gets a refusal, not a form, when the new-channel view is opened by its address', async () => {
    const view = mount(`${PAGE_URL}?slide=notification-channel-new`, role);
    await findPage(PROJECT_NAME);
    expect(screen.queryByLabelText(Ar['notifications.form.name'])).not.toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });
});

describe('a maintainer on the notifications page', () => {
  it('sees the buttons and the delivery log', async () => {
    mount(PAGE_URL, 'maintainer');
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByRole('button', { name: Ar['notifications.channels.new'] })).toBeInTheDocument();
    expect(await within(main).findByText(Ar['notifications.deliveries.title'], { selector: 'h3' })).toBeInTheDocument();
  });

  it('cannot create a channel in an archived project', async () => {
    mount(PAGE_URL, 'owner', undefined, { State: 'archived' });
    const main = await findPage(PROJECT_NAME);
    expect(await channelRow(main, 'ops-hook')).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['notifications.channels.new'] })).not.toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['notifications.test.send'] })).not.toBeInTheDocument();
  });

  it('opens the project tab from the project navigation', async () => {
    const view = mount(`/projects/${PROJECT_ID}/ingest`);
    await findPage(PROJECT_NAME);
    await view.user.click(await screen.findByRole('link', { name: Ar['projects.nav.notifications'] }));
    expect(await screen.findByText(Ar['notifications.channels.title'])).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe(`/projects/${PROJECT_ID}/notifications`);
  });
});

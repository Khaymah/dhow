import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { CHANNEL_ID, NEW_SECRET } from '@/Core/Testing/NotificationsBackend';
import { API, cachedText, fillChannel, mount, named, openNew, PAGE_URL, PASSWORD, PROJECT_NAME, storedText, submit, typePassword } from '@/App/NotificationsHarness';
import { findPage } from '@/Core/Testing/Pages';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

const makeWebhook = async (view: ReturnType<typeof mount>) => {
  const dialog = await openNew(view);
  await fillChannel(view, dialog, 'webhook', 'release-hook', 'https://hooks.example.com/dhow');
  await typePassword(view, dialog);
  await submit(view, dialog);
  return dialog;
};

describe('making a channel', () => {
  it('sends the kind, the name, the address and the second step, then shows the signing secret once', async () => {
    const view = mount();
    const dialog = await makeWebhook(view);

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/notification-channels`,
      Body: {
        Kind: 'webhook',
        Name: 'release-hook',
        Locale: 'ar',
        Url: 'https://hooks.example.com/dhow',
        CurrentFactor: { Method: 'password', Password: PASSWORD },
      },
    });
    const value = await within(dialog).findByLabelText(Ar['notifications.secret.value']);
    expect(value).toHaveTextContent(NEW_SECRET);
    expect(within(dialog).getByText(Ar['notifications.secret.title'])).toBeInTheDocument();
    expect(view.router.state.location.href).toBe(`${PAGE_URL}?slide=notification-channel-new`);
    expect(view.router.state.location.href).not.toContain(NEW_SECRET);
    expect(storedText()).not.toContain(NEW_SECRET);
    expect(cachedText(view)).not.toContain(NEW_SECRET);
  });

  it('does not send anything without the second step, and asks for it', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillChannel(view, dialog, 'webhook', 'release-hook', 'https://hooks.example.com/dhow');
    await submit(view, dialog);

    expect(await within(dialog).findByText(Ar['factor.error.required'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
    expect(within(dialog).queryByLabelText(Ar['notifications.secret.value'])).not.toBeInTheDocument();
  });

  it('checks the form before it asks the server', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByText(Ar['notifications.form.nameInvalid'])).toBeInTheDocument();

    await fillChannel(view, dialog, 'webhook', 'release-hook', 'http://hooks.example.com/dhow');
    await submit(view, dialog);
    expect(await within(dialog).findByText(Ar['notifications.form.urlInvalid'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(0);
  });

  it('shows no secret for a channel that has none, and closes the view', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillChannel(view, dialog, 'inapp', 'inbox');
    await typePassword(view, dialog);
    await submit(view, dialog);

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(view.state.writes[0]?.Body).toMatchObject({ Kind: 'inapp', Name: 'inbox' });
    expect(view.state.writes[0]?.Body).not.toHaveProperty('Url');
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText('inbox')).toBeInTheDocument();
  });

  it('does not show a secret when the server refuses', async () => {
    const view = mount();
    view.state.failCreate = new Response(JSON.stringify({ Success: false, Error: { Code: 'CHANNEL_EXISTS', Message: 'الاسم مستخدم' } }), {
      status: 409,
      headers: { 'Content-Type': 'application/json' },
    });
    const dialog = await makeWebhook(view);
    expect(await within(dialog).findByText('الاسم مستخدم')).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(Ar['notifications.secret.value'])).not.toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain(NEW_SECRET);
  });
});

describe('the signing secret', () => {
  it('copies the secret and clears it everywhere when the person is done', async () => {
    const view = mount();
    const dialog = await makeWebhook(view);
    await within(dialog).findByLabelText(Ar['notifications.secret.value']);

    await view.user.click(within(dialog).getByRole('button', { name: Ar['notifications.secret.copy'] }));
    expect(await navigator.clipboard.readText()).toBe(NEW_SECRET);
    expect(await screen.findByText(Ar['notifications.secret.copied'])).toBeInTheDocument();
    expect(JSON.stringify(useToastStore.getState().Toasts)).not.toContain(NEW_SECRET);

    await view.user.click(within(dialog).getByRole('button', { name: Ar['notifications.secret.done'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(document.body.innerHTML).not.toContain(NEW_SECRET);
    expect(storedText()).not.toContain(NEW_SECRET);
    expect(cachedText(view)).not.toContain(NEW_SECRET);
    expect(view.router.state.location.href).toBe(PAGE_URL);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText('release-hook')).toBeInTheDocument();
  });

  it('clears the secret when the person leaves the page without closing the view', async () => {
    const view = mount();
    const dialog = await makeWebhook(view);
    await within(dialog).findByLabelText(Ar['notifications.secret.value']);

    await view.router.navigate({ to: '/projects/$projectId/environments', params: { projectId: PROJECT_ID } });
    await waitFor(() => {
      expect(screen.queryByLabelText(Ar['notifications.secret.value'])).not.toBeInTheDocument();
    });
    expect(document.body.innerHTML).not.toContain(NEW_SECRET);
    expect(storedText()).not.toContain(NEW_SECRET);

    await view.router.navigate({ to: '/projects/$projectId/notifications', params: { projectId: PROJECT_ID }, search: { slide: 'notification-channel-new' } });
    const again = await screen.findByRole('dialog', { name: Ar['notifications.new.title'] });
    expect(within(again).queryByLabelText(Ar['notifications.secret.value'])).not.toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain(NEW_SECRET);
    expect(cachedText(view)).not.toContain(NEW_SECRET);
  });

  it('shows a new secret once after a rotation, then wipes it', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    await view.user.click(await within(main).findByRole('button', { name: named(Ar['notifications.rotate.aria'], 'ops-hook') }));
    const panel = await within(main).findByText(named(Ar['notifications.rotate.question'], 'ops-hook'));
    const row = panel.closest('li');
    if (row === null) throw new Error('the panel is not inside a channel row');
    await view.user.click(await within(row).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(row).getByRole('button', { name: Ar['notifications.rotate.confirm'] }));

    expect(await within(row).findByLabelText(Ar['notifications.secret.value'])).toHaveTextContent(NEW_SECRET);
    expect(view.state.writes[0]).toMatchObject({
      Method: 'PUT',
      Path: `${API}/notification-channels/${CHANNEL_ID}`,
      Body: { RotateSecret: true, Name: 'ops-hook', Version: 1 },
    });
    expect(cachedText(view)).not.toContain(NEW_SECRET);
    expect(storedText()).not.toContain(NEW_SECRET);

    await view.user.click(within(row).getByRole('button', { name: Ar['notifications.secret.done'] }));
    await waitFor(() => {
      expect(document.body.innerHTML).not.toContain(NEW_SECRET);
    });
    expect(cachedText(view)).not.toContain(NEW_SECRET);
  });
});

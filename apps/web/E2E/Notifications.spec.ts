import { expect, test, type Locator } from '@playwright/test';

import { DESKTOP, expectClean, expectNowhere, open, shot, signInPage, type IView } from './Guard';
import { call, invite, signIn, stack, type ISession, type IUser } from './Stack';

test.describe.configure({ mode: 'serial' });

const RECEIVER = process.env.DHOW_E2E_NOTIFY_RECEIVER_URL ?? '';
const SECRET = /^dhow_whsec_/;

let owner: ISession;
let developer: IUser;

const channelsPath = () => `/projects/${stack().ProjectId}/notifications`;

const close = async (view: IView) => {
  await expectClean(view);
  await view.Context.close();
};

const typePassword = async (dialog: Locator, view: IView, password = stack().Owner.Password) => {
  await dialog.getByRole('textbox', { name: view.T['factor.method.password'], exact: true }).fill(password);
};

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
  const invited: IUser = { Email: 'notify-dev@dhow-demo.test', Password: 'a long notify developer passphrase', Name: 'Nour Developer' };
  await invite(owner, invited, 'developer');
  developer = invited;
});

test('the owner makes an in-app channel, sends a test message and finds it in the inbox with the unread badge', async ({ browser }) => {
  const view = await open(browser, 'en', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(channelsPath());

  await page.getByRole('button', { name: T['notifications.channels.new'] }).click();
  const dialog = page.getByRole('dialog', { name: T['notifications.new.title'] });
  await dialog.getByLabel(T['notifications.form.kind']).selectOption('inapp');
  await dialog.getByRole('textbox', { name: T['notifications.form.name'] }).fill('e2e-inbox');
  await typePassword(dialog, view);
  await dialog.getByRole('button', { name: T['notifications.form.submit'] }).click();
  await expect(dialog).toBeHidden();

  const channels = page.getByRole('list', { name: T['notifications.channels.title'] });
  const row = channels.getByRole('listitem').filter({ hasText: 'e2e-inbox' });
  await expect(row).toBeVisible();
  await row.getByRole('button', { name: T['notifications.test.send'] }).click();

  await page.goto('/notifications');
  const inbox = page.getByRole('list', { name: T['notifications.title'] });
  await expect(inbox.getByText(T['notifications.inbox.notification.test']).first()).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('button', { name: /unread/ })).toBeVisible();
  await shot(view, test.info().outputPath('inbox-en.png'));

  await page.getByRole('button', { name: T['notifications.inbox.markAll'] }).click();
  await expect(page.getByRole('button', { name: T['shell.notifications'], exact: true })).toBeVisible();
  await close(view);
});

test('a developer reads the channels and has no button to create one, and the server refuses the call', async ({ browser }) => {
  const session = await signIn(developer);
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, developer, session);
  await page.goto(channelsPath());

  await expect(page.getByRole('list', { name: T['notifications.channels.title'] })).toBeVisible();
  await expect(page.getByRole('button', { name: T['notifications.channels.new'] })).toHaveCount(0);
  await expect(page.getByText(T['notifications.readOnly'])).toBeVisible();

  await expect(
    call(
      'POST',
      `/organizations/${stack().OrgId}/projects/${stack().ProjectId}/notification-channels`,
      { Kind: 'inapp', Name: 'dev-try', CurrentFactor: { Method: 'password', Password: developer.Password } },
      session,
    ),
  ).rejects.toThrow(/answered 403: .*"Code":"FORBIDDEN"/);
  await close(view);
});

test.describe('with the local notification receiver', () => {
  test.skip(RECEIVER === '', 'set DHOW_E2E_NOTIFY_RECEIVER_URL to the https address of deploy/e2e-targets/notify-receiver, trusted by the server');

  let secret = '';

  test('a webhook channel shows its signing secret once and the test message is delivered and logged', async ({ browser }) => {
    const view = await open(browser, 'en', DESKTOP);
    const { Page: page, T } = view;
    await signInPage(view, stack().Owner, owner);
    await page.goto(channelsPath());

    await page.getByRole('button', { name: T['notifications.channels.new'] }).click();
    const dialog = page.getByRole('dialog', { name: T['notifications.new.title'] });
    await dialog.getByLabel(T['notifications.form.kind']).selectOption('webhook');
    await dialog.getByRole('textbox', { name: T['notifications.form.name'] }).fill('e2e-hook');
    await dialog.getByLabel(T['notifications.form.url']).fill(RECEIVER);
    await typePassword(dialog, view);
    await dialog.getByRole('button', { name: T['notifications.form.submit'] }).click();

    const box = dialog.getByLabel(T['notifications.secret.value']);
    await expect(box).toBeVisible();
    secret = (await box.textContent()) ?? '';
    expect(secret).toMatch(SECRET);
    await dialog.getByRole('button', { name: T['notifications.secret.done'] }).click();
    await expect(dialog).toBeHidden();
    await expectNowhere(view, secret);
    await page.reload();
    await expectNowhere(view, secret);

    const row = page.getByRole('list', { name: T['notifications.channels.title'] }).getByRole('listitem').filter({ hasText: 'e2e-hook' });
    await row.getByRole('button', { name: T['notifications.test.send'] }).click();
    const log = page.getByRole('list', { name: T['notifications.deliveries.title'] });
    await expect(log.getByRole('listitem').filter({ hasText: 'e2e-hook' }).getByText(T['notifications.deliveries.state.sent'])).toBeVisible({
      timeout: 60_000,
    });
    await shot(view, test.info().outputPath('delivery-log-en.png'));
    await close(view);
  });
});

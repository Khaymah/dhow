import { expect, test, type Locator } from '@playwright/test';

import { digestOf, tarGz, upload } from './Archive';
import { DESKTOP, expectClean, expectNowhere, open, PHONE, shot, signInPage, type IView } from './Guard';
import { call, invite, signIn, stack, text, type ISession, type IUser } from './Stack';

test.describe.configure({ mode: 'serial' });

const PROTECTED = 'ingest-gate';
const DOCS_TOKEN = 'e2e-docs';
const WIDE_TOKEN = 'e2e-wide';
const FILE_NAME = 'e2e-site';
const SHA = '0123456789abcdef0123456789abcdef01234567';

let owner: ISession;
let developer: IUser;
let docsValue = '';

const ingestPath = () => `/projects/${stack().ProjectId}/ingest`;

const close = async (view: IView) => {
  await expectClean(view);
  await view.Context.close();
};

const fillToken = async (dialog: Locator, view: IView, name: string, pattern: string) => {
  const { T } = view;
  await dialog.getByRole('textbox', { name: T['ingest.form.name'] }).fill(name);
  await dialog.getByRole('textbox', { name: T['ingest.patterns.pattern'] }).fill(pattern);
};

const typePassword = async (dialog: Locator, view: IView) => {
  await dialog.getByRole('textbox', { name: view.T['factor.method.password'], exact: true }).fill(stack().Owner.Password);
};

const valueOf = async (dialog: Locator, view: IView): Promise<string> => {
  const box = dialog.getByLabel(view.T['ingest.created.value']);
  await expect(box).toBeVisible();
  const value = (await box.textContent()) ?? '';
  expect(value).toMatch(/^dhow_ing_/);
  return value;
};

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
  const invited: IUser = { Email: 'ingest-dev@dhow-demo.test', Password: 'a long ingest developer passphrase', Name: 'Dina Developer' };
  await invite(owner, invited, 'developer');
  developer = invited;
  const reviewer = await signIn(invited);
  const reviewerId = text((await call('GET', '/me', undefined, reviewer)).Data, 'User', 'Id');
  await call(
    'POST',
    `/organizations/${stack().OrgId}/projects/${stack().ProjectId}/environments`,
    {
      Name: PROTECTED,
      Description: '',
      Rules: { AllowedRefs: [{ Kind: 'branch', Pattern: 'main' }], Reviewers: { UserIds: [reviewerId], TeamIds: [] } },
      CurrentFactor: { Method: 'password', Password: stack().Owner.Password },
    },
    owner,
  );
});

test('the owner makes a token that reaches nothing protected and sees its value once in Arabic', async ({ browser }) => {
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);

  await page.goto(`/projects/${stack().ProjectId}/settings`);
  await page.getByRole('link', { name: new RegExp(T['ingest.settings.link']) }).click();
  await expect(page).toHaveURL(new RegExp(`${ingestPath()}$`));
  const create = page.getByRole('button', { name: T['ingest.tokens.new'] });
  await expect(create).toBeVisible();

  await create.click();
  const dialog = page.getByRole('dialog', { name: T['ingest.new.title'] });
  await fillToken(dialog, view, DOCS_TOKEN, 'e2e/*');
  await typePassword(dialog, view);
  await dialog.getByRole('button', { name: T['ingest.form.submit'] }).click();

  docsValue = await valueOf(dialog, view);
  await expect(dialog.getByText(T['ingest.created.lead'])).toBeVisible();
  await expect(dialog.getByText(T['ingest.reach.title'])).toHaveCount(0);
  await shot(view, test.info().outputPath('token-created-ar.png'));

  await dialog.getByRole('button', { name: T['ingest.created.done'] }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByText(DOCS_TOKEN, { exact: true })).toBeVisible();
  await expectNowhere(view, docsValue);
  await page.reload();
  await expect(page.getByText(DOCS_TOKEN, { exact: true })).toBeVisible();
  await expectNowhere(view, docsValue);
  await close(view);
});

test('a file uploaded with the token shows in the list, declared and not verified', async ({ browser }) => {
  const body = tarGz({ 'index.html': '<h1>hello</h1>' });
  const sent = await upload(docsValue, body, FILE_NAME, 'e2e/build', SHA);
  expect(sent.Status).toBe(201);
  expect(sent.Digest).toBe(digestOf(body));

  const view = await open(browser, 'en', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(ingestPath());

  const files = page.getByRole('list', { name: T['ingest.files.title'] });
  await expect(files.getByText(FILE_NAME, { exact: true })).toBeVisible();
  await expect(files.getByText(`sha256:${sent.Digest.slice(0, 12)}`)).toBeVisible();
  await expect(files.getByText(T['ingest.file.unverified'])).toBeVisible();
  await expect(files.getByText('e2e/build')).toBeVisible();
  await expect(files.getByText(SHA.slice(0, 8))).toBeVisible();
  await expect(files.getByText(T['ingest.file.noRun'])).toBeVisible();
  await shot(view, test.info().outputPath('files-en.png'));
  await expectNowhere(view, docsValue);
  await close(view);
});

test('a wide pattern lists the protected environment and needs an explicit yes, and the value goes the moment the slide closes', async ({ browser }) => {
  const view = await open(browser, 'en', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(ingestPath());

  await page.getByRole('button', { name: T['ingest.tokens.new'] }).click();
  const dialog = page.getByRole('dialog', { name: T['ingest.new.title'] });
  await fillToken(dialog, view, WIDE_TOKEN, '**');
  await typePassword(dialog, view);
  await dialog.getByRole('button', { name: T['ingest.form.submit'] }).click();

  await expect(dialog.getByText(T['ingest.reach.title'])).toBeVisible();
  await expect(dialog.getByRole('list', { name: T['ingest.reach.label'] }).getByText(PROTECTED)).toBeVisible();
  await expect(dialog.getByLabel(T['ingest.created.value'])).toHaveCount(0);
  const submit = dialog.getByRole('button', { name: T['ingest.form.submit'] });
  await expect(submit).toBeDisabled();
  await shot(view, test.info().outputPath('reach-en.png'));

  await dialog.getByRole('checkbox', { name: new RegExp(T['ingest.reach.acknowledge']) }).click();
  await expect(submit).toBeEnabled();
  await typePassword(dialog, view);
  await submit.click();

  const value = await valueOf(dialog, view);
  expect(value).not.toBe(docsValue);
  await expect(dialog.getByRole('list', { name: T['ingest.reach.label'] }).getByText(PROTECTED)).toBeVisible();

  await page.keyboard.press('Escape');
  await expectNowhere(view, value);
  await expect(dialog).toBeHidden();
  await expectNowhere(view, value);
  await expect(page.getByText(WIDE_TOKEN, { exact: true })).toBeVisible();
  await expect(page.getByRole('list', { name: T['ingest.tokens.title'] }).getByText(PROTECTED).first()).toBeVisible();
  await page.goto(`/projects/${stack().ProjectId}/environments`);
  await expectNowhere(view, value);
  await close(view);
});

test('the owner revokes a token on a phone with the second step and the old value stops working', async ({ browser }) => {
  const view = await open(browser, 'ar', PHONE);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(ingestPath());

  const revoke = page.getByRole('button', { name: new RegExp(DOCS_TOKEN) });
  await revoke.click();
  const confirm = page.getByRole('button', { name: T['ingest.revoke.confirm'] });
  const password = page.getByRole('textbox', { name: T['factor.method.password'], exact: true });
  const panel = page.locator('div').filter({ has: confirm }).filter({ has: password }).last();
  await panel.getByRole('textbox', { name: T['factor.method.password'], exact: true }).fill(stack().Owner.Password);
  await panel.getByRole('button', { name: T['ingest.revoke.confirm'] }).click();

  await expect(revoke).toHaveCount(0);
  await expect(page.getByText(T['ingest.token.status.revoked']).first()).toBeVisible();
  await shot(view, test.info().outputPath('revoked-ar-390.png'));

  const late = await upload(docsValue, tarGz({ 'index.html': '<h1>again</h1>' }), FILE_NAME, 'e2e/build', SHA);
  expect(late.Status).toBe(401);
  expect(late.Code).toBe('INGEST_TOKEN_REVOKED');
  await expectNowhere(view, docsValue);
  await close(view);
});

test('a developer reads the received files and has no token controls', async ({ browser }) => {
  const view = await open(browser, 'en', DESKTOP);
  const { Page: page, T } = view;
  const session = await signIn(developer);
  await signInPage(view, developer, session);
  await page.goto(ingestPath());

  await expect(page.getByRole('list', { name: T['ingest.files.title'] }).getByText(FILE_NAME, { exact: true })).toBeVisible();
  await expect(page.getByText(T['ingest.readOnly'])).toBeVisible();
  await expect(page.getByRole('button', { name: T['ingest.tokens.new'] })).toHaveCount(0);
  await expect(page.getByRole('list', { name: T['ingest.tokens.title'] })).toHaveCount(0);
  await expectNowhere(view, docsValue);
  await close(view);
});

import { expect, test } from '@playwright/test';

import { DESKTOP, expectClean, expectNowhere, open, PHONE, shot, signInPage, type IView } from './Guard';
import { freshCode, invite, push, runOn, signIn, stack, turnOnTotp, waitRun, type ISession, type IUser } from './Stack';

test.describe.configure({ mode: 'serial' });

const ENVIRONMENT = 'production';
const SECRET_NAME = 'DEPLOY_TOKEN';
const SECRET_VALUE = `e2e-secret-${Date.now().toString(36)}-value`;
const FEATURE = `feature/e2e-${Date.now().toString(36)}`;

let owner: ISession;
let reviewer: IUser;
let reviewerSession: ISession;
let skippedRun = { Id: '', Number: 0 };
let waitingRun = { Id: '', Number: 0 };

const runPath = (id: string) => `/projects/${stack().ProjectId}/runs/${id}`;
const environmentPath = () => `/projects/${stack().ProjectId}/environments/${ENVIRONMENT}`;

const close = async (view: IView) => {
  await expectClean(view);
  await view.Context.close();
};

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
  const invited: IUser = { Email: 'reviewer@dhow-demo.test', Password: 'a long reviewer passphrase', Name: 'Rana Reviewer' };
  await invite(owner, invited, 'developer');
  const enrolled = await turnOnTotp(invited);
  reviewer = enrolled.User;
  reviewerSession = enrolled.Session;
});

test('the owner makes the environment with its rules and its secret in Arabic', async ({ browser }) => {
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);

  await page.goto(`/projects/${stack().ProjectId}/environments`);
  await page.getByRole('button', { name: T['environments.add'] }).click();
  const create = page.getByRole('dialog');
  await create.getByRole('textbox', { name: T['environments.form.name'] }).fill(ENVIRONMENT);
  await create.getByRole('button', { name: T['environments.refs.add'] }).click();
  await create.getByRole('textbox', { name: T['environments.refs.pattern'] }).fill('main');
  await create.getByRole('checkbox', { name: new RegExp(reviewer.Name) }).check();
  await create.getByRole('textbox', { name: T['factor.method.password'], exact: true }).fill(stack().Owner.Password);
  await create.getByRole('button', { name: T['environments.form.submit'] }).click();
  await expect(create).toBeHidden();

  await page.goto(environmentPath());
  await page.getByRole('button', { name: T['secrets.add'] }).click();
  const secret = page.getByRole('dialog');
  await secret.getByRole('textbox', { name: T['secrets.form.name'] }).fill(SECRET_NAME);
  await secret.getByRole('textbox', { name: T['secrets.form.value'] }).fill(SECRET_VALUE);
  await secret.getByRole('button', { name: T['secrets.form.submit'] }).click();
  await expect(secret).toBeHidden();
  await expect(page.getByText(SECRET_NAME, { exact: true })).toBeVisible();
  await expectNowhere(view, SECRET_VALUE);
  await shot(view, test.info().outputPath('environment-ar.png'));
  await close(view);
});

test('a push on a branch the environment refuses is skipped with the reason', async ({ browser }) => {
  await push(FEATURE);
  skippedRun = await runOn(owner, FEATURE, 0);
  await waitRun(owner, skippedRun.Id, ['succeeded', 'skipped', 'failed', 'canceled']);
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(runPath(skippedRun.Id));
  await expect(page.getByText(T['approvals.gate.skipped.badge'])).toBeVisible();
  await expect(page.getByText(FEATURE).first()).toBeVisible();
  await shot(view, test.info().outputPath('skipped-ar.png'));
  await close(view);
});

test('a push on main waits for approval and the owner, who is no reviewer, cannot approve it', async ({ browser }) => {
  await push('main');
  waitingRun = await runOn(owner, 'main', skippedRun.Number);
  await waitRun(owner, waitingRun.Id, ['awaiting_approval']);
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(runPath(waitingRun.Id));
  await expect(page.getByText(T['approvals.gate.approval.badge']).first()).toBeVisible();
  await expect(page.getByRole('button', { name: T['approvals.gate.approve'] })).toHaveCount(0);
  await expectNowhere(view, SECRET_VALUE);
  await shot(view, test.info().outputPath('waiting-ar.png'));
  await close(view);
});

test('the reviewer approves with an authenticator code on a phone in English and the job finishes', async ({ browser }) => {
  const view = await open(browser, 'en', PHONE);
  const { Page: page, T } = view;
  await signInPage(view, reviewer, reviewerSession);

  await page.goto('/approvals');
  await expect(page.getByRole('heading', { name: T['approvals.page.title'] })).toBeVisible();
  await expect(page.getByText(ENVIRONMENT).first()).toBeVisible();
  await shot(view, test.info().outputPath('approvals-en-390.png'));

  await page.goto(runPath(waitingRun.Id));
  await page.getByRole('button', { name: T['approvals.gate.approve'] }).click();
  const decide = page.getByRole('dialog', { name: T['approvals.decide.approve.title'] });
  await expect(decide.getByRole('textbox', { name: T['factor.method.password'], exact: true })).toHaveCount(0);
  await decide.getByRole('textbox', { name: T['factor.method.totp'] }).fill(await freshCode(reviewer.TotpSecret ?? ''));
  await decide.getByRole('button', { name: T['approvals.decide.approve.submit'] }).click();
  await expect(decide).toBeHidden();

  expect(await waitRun(reviewerSession, waitingRun.Id, ['succeeded', 'failed', 'canceled'])).toBe('succeeded');
  await page.reload();
  await expect(page.getByText(T['jobs.state.succeeded']).first()).toBeVisible();
  await expectNowhere(view, SECRET_VALUE);
  await shot(view, test.info().outputPath('approved-en-390.png'));
  await close(view);
});

test('the run and the environment read well in English on a desktop and in Arabic on a phone', async ({ browser }) => {
  const pages = [
    { lang: 'en' as const, viewport: DESKTOP, name: 'en-1440' },
    { lang: 'ar' as const, viewport: PHONE, name: 'ar-390' },
  ];
  for (const entry of pages) {
    const view = await open(browser, entry.lang, entry.viewport);
    const { Page: page, T } = view;
    await signInPage(view, stack().Owner, owner);
    await page.goto(runPath(waitingRun.Id));
    await expect(page.getByText(T['jobs.state.succeeded']).first()).toBeVisible();
    await shot(view, test.info().outputPath(`run-${entry.name}.png`));
    await page.goto(runPath(skippedRun.Id));
    await expect(page.getByText(T['approvals.gate.skipped.badge'])).toBeVisible();
    await page.goto(environmentPath());
    await expect(page.getByText(SECRET_NAME, { exact: true })).toBeVisible();
    await expectNowhere(view, SECRET_VALUE);
    await shot(view, test.info().outputPath(`environment-${entry.name}.png`));
    await close(view);
  }
});

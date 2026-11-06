import { expect, test } from '@playwright/test';

import type { components } from '../Src/Core/Api/Schema';
import { DESKTOP, expectClean, expectNowhere, open, PHONE, shot, signInPage, type IView } from './Guard';
import { newHostKey } from './HostKey';
import { call, field, list, signIn, stack, text, type ISession } from './Stack';

test.describe.configure({ mode: 'serial' });

const ENVIRONMENT = 'targets-e2e';
const TARGET = 'api';
const ADDRESS = 'deploy.example.test';
const KEY_SECRET = 'API_SSH_KEY';
const KEY_MATERIAL = 'e2e-fake-deploy-key-material-0123456789';
const HOST_KEY = newHostKey(ADDRESS);

let owner: ISession;

const environmentPath = () => `/projects/${stack().ProjectId}/environments/${ENVIRONMENT}`;
const targetPath = () => `${environmentPath()}/targets/${TARGET}`;
const apiPath = () => `/organizations/${stack().OrgId}/projects/${stack().ProjectId}/environments/${ENVIRONMENT}/targets/${TARGET}`;
const factor = () => ({ Method: 'password', Password: stack().Owner.Password });

const close = async (view: IView) => {
  await expectClean(view);
  await view.Context.close();
};

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
  const { OrgId, ProjectId } = stack();
  await call('POST', `/organizations/${OrgId}/projects/${ProjectId}/environments`, { Name: ENVIRONMENT, CurrentFactor: factor() }, owner);
});

test('the owner adds a target from the environment page and lands on its page with the key not pinned', async ({ browser }) => {
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);

  await page.goto(environmentPath());
  await expect(page.getByRole('region', { name: T['targets.group'] })).toBeVisible();
  await expect(page.getByText(T['targets.empty'])).toBeVisible();
  await page.getByRole('button', { name: T['targets.add'] }).click();

  await expect(page.getByRole('heading', { level: 1, name: T['targets.new.title'] })).toBeVisible();
  await page.getByRole('textbox', { name: T['targets.form.name'] }).fill(TARGET);
  await page.getByRole('textbox', { name: T['targets.form.path'] }).fill('/srv/api');
  await page.getByRole('textbox', { name: T['targets.form.host'] }).fill(ADDRESS);
  await page.getByRole('textbox', { name: T['factor.method.password'], exact: true }).fill(stack().Owner.Password);
  await page.getByRole('button', { name: T['targets.form.create'] }).click();

  await expect(page.getByRole('heading', { level: 1, name: TARGET })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`${targetPath()}$`));
  await expect(page.getByText(T['targets.hostkey.badge.unpinned'], { exact: true })).toBeVisible();
  await expect(page.getByText(T['targets.hostkey.unpinned'])).toBeVisible();
  await shot(view, test.info().outputPath('target-new-ar.png'));

  await page.getByRole('link', { name: T['targets.page.back'] }).click();
  await expect(page.getByRole('heading', { level: 3, name: TARGET })).toBeVisible();
  await expect(page.getByRole('link', { name: T['targets.manage'] })).toBeVisible();
  await close(view);
});

test('a host key is pinned only after its fingerprint is shown, compared and confirmed with the password', async ({ browser }) => {
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(targetPath());

  await page.getByRole('button', { name: T['targets.hostkey.paste'] }).click();
  await page.getByRole('textbox', { name: T['targets.hostkey.paste.label'] }).fill(HOST_KEY.Line);
  await page.getByRole('button', { name: T['targets.hostkey.paste.submit'] }).click();

  await expect(page.getByText(T['targets.hostkey.pending.title'])).toBeVisible();
  await expect(page.getByText(HOST_KEY.Fingerprint, { exact: true })).toBeVisible();
  const confirm = page.getByRole('button', { name: T['targets.hostkey.pending.confirm'] });
  await expect(confirm).toBeDisabled();
  await expect(page.getByText(T['targets.hostkey.badge.unpinned'], { exact: true })).toBeVisible();

  await page.getByRole('checkbox', { name: T['targets.hostkey.pending.check'] }).check();
  const password = page.getByRole('textbox', { name: T['factor.method.password'], exact: true });
  const panel = page.locator('div').filter({ has: confirm }).filter({ has: password }).last();
  await expect(panel.getByText(ADDRESS, { exact: false })).toBeVisible();
  await panel.getByRole('textbox', { name: T['factor.method.password'], exact: true }).fill(stack().Owner.Password);
  await expect(confirm).toBeEnabled();
  await shot(view, test.info().outputPath('host-key-confirm-ar.png'));
  await confirm.click();

  await expect(page.getByText(T['targets.hostkey.badge.pinned'], { exact: true })).toBeVisible();
  await expect(page.getByText(HOST_KEY.Fingerprint, { exact: true })).toBeVisible();
  await expect(page.getByText(T['targets.hostkey.pending.title'])).toBeHidden();

  const saved = (await call('GET', apiPath(), undefined, owner)).Data;
  const hosts = list(saved, 'Hosts');
  expect(hosts).toHaveLength(1);
  expect(text(hosts[0], 'HostKeyFingerprint')).toBe(HOST_KEY.Fingerprint);
  expect(field(hosts[0], 'KeyPinned')).toBe(true);
  await close(view);
});

test('the private key goes into the environment secret once and is never shown again', async ({ browser }) => {
  const view = await open(browser, 'ar', DESKTOP);
  const { Page: page, T } = view;
  await signInPage(view, stack().Owner, owner);
  await page.goto(targetPath());

  await page.getByRole('textbox', { name: T['targets.keySecret.value'], exact: true }).fill(KEY_MATERIAL);
  await page.getByRole('button', { name: T['targets.keySecret.submit'] }).click();
  await expect(page.getByRole('textbox', { name: T['targets.keySecret.value'], exact: true })).toHaveValue('');

  await expect(page.getByText(KEY_SECRET, { exact: false }).first()).toBeVisible();
  const attach = page.getByRole('button', { name: T['targets.keySecret.attach.confirm'] });
  const attachPassword = page.getByRole('textbox', { name: T['factor.method.password'], exact: true });
  const panel = page.locator('div').filter({ has: attach }).filter({ has: attachPassword }).last();
  await panel.getByRole('textbox', { name: T['factor.method.password'], exact: true }).fill(stack().Owner.Password);
  await panel.getByRole('button', { name: T['targets.keySecret.attach.confirm'] }).click();

  await expect(page.getByText(T['targets.key.secret'].replace('{name}', KEY_SECRET))).toBeVisible();
  await expectNowhere(view, KEY_MATERIAL);
  const saved = (await call('GET', apiPath(), undefined, owner)).Data;
  expect(text(saved, 'KeySecretName')).toBe(KEY_SECRET);
  expect(JSON.stringify(saved)).not.toContain(KEY_MATERIAL);
  const listed = await call('GET', `/organizations/${stack().OrgId}/projects/${stack().ProjectId}/environments/${ENVIRONMENT}/secrets`, undefined, owner);
  expect(JSON.stringify(listed.Data)).not.toContain(KEY_MATERIAL);
  await close(view);
});

test('the history renders a rolled-back release (served by a stub of the releases endpoint) with the reason and each server', async ({ browser }) => {
  const saved = (await call('GET', apiPath(), undefined, owner)).Data;
  const hostId = text(list(saved, 'Hosts')[0], 'Id');
  const releases: components['schemas']['DeployTargetReleases'] = {
    CurrentReleaseId: '1-aaaaaaaa-1',
    Partial: false,
    Releases: [
      {
        Id: '0198a000-0000-7000-8000-00000000d002',
        ReleaseId: '2-bbbbbbbb-1',
        Kind: 'deploy',
        State: 'rolled_back',
        PreviousReleaseId: '1-aaaaaaaa-1',
        RollbackReason: 'health_check_failed',
        Message: 'the health check answered 503',
        StartedAt: new Date(Date.now() - 600_000).toISOString(),
        FinishedAt: new Date(Date.now() - 540_000).toISOString(),
        RollbackTarget: false,
        ArtifactKept: true,
        Hosts: [{ HostId: hostId, Position: 1, State: 'rolled_back', RollbackReason: 'health_check_failed' }],
      },
      {
        Id: '0198a000-0000-7000-8000-00000000d001',
        ReleaseId: '1-aaaaaaaa-1',
        Kind: 'deploy',
        State: 'succeeded',
        StartedAt: new Date(Date.now() - 3_600_000).toISOString(),
        FinishedAt: new Date(Date.now() - 3_500_000).toISOString(),
        RollbackTarget: true,
        ArtifactKept: true,
        Hosts: [{ HostId: hostId, Position: 1, State: 'verified' }],
      },
    ],
  };

  for (const [lang, viewport] of [
    ['ar', DESKTOP],
    ['en', PHONE],
  ] as const) {
    const view = await open(browser, lang, viewport);
    const { Page: page, T } = view;
    await page.route(`**/api/v1${apiPath()}/releases`, (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ Success: true, Data: releases }) }),
    );
    await signInPage(view, stack().Owner, owner);
    await page.goto(targetPath());

    const row = page.getByRole('listitem').filter({ hasText: '2-bbbbbbbb-1' });
    await expect(row.getByText(T['targets.release.auto'])).toBeVisible();
    await expect(row.getByText(T['targets.release.reason'].replace('{reason}', T['targets.reason.health_check_failed']), { exact: false })).toBeVisible();
    await expect(row.getByText(`${ADDRESS}:22`)).toBeVisible();
    await expect(row.getByRole('button', { name: T['targets.rollback.action'] })).toHaveCount(0);
    await expect(page.getByRole('listitem').filter({ hasText: '1-aaaaaaaa-1' }).getByText(T['targets.release.auto'])).toHaveCount(0);
    await shot(view, test.info().outputPath(`history-${lang}.png`));
    await close(view);
  }
});

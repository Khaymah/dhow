import { expect, test, type Browser, type Page } from '@playwright/test';

import { focusCovered, scan, THEMES, type IFinding, type TThemeId } from './Axe';
import { boardOf, stubBoard } from './BoardFixture';
import { DESKTOP, expectClean, open, PHONE, signInPage, type TLang } from './Guard';
import { lineText, stubLog } from './LogFixture';
import { pagePlaces, slidePlaces, type IPlace, type IPlaceContext, type TStub } from './Places';
import { call, field, list, push, runOn, setTheme, signIn, stack, text, type ISession } from './Stack';

test.describe.configure({ mode: 'serial' });

const LANGS: TLang[] = ['ar', 'en'];
const ENVIRONMENT = 'a11y-check';
const LOG_LINES = 20;
const SWEEP_TIMEOUT_MS = 900_000;
const PHONE_THEME: TThemeId = THEMES[0];

let owner: ISession;
let environment = ENVIRONMENT;
let runId = '';

const projectBase = () => `/organizations/${stack().OrgId}/projects/${stack().ProjectId}`;

const context = (): IPlaceContext => ({ ProjectId: stack().ProjectId, Environment: environment, RunId: runId });

const GROUPS = ['pages', 'slides'] as const;
type TGroup = (typeof GROUPS)[number];

const placesOf = (group: TGroup): IPlace[] => (group === 'pages' ? pagePlaces(context()) : slidePlaces(context()));

const describeFindings = (findings: IFinding[]): string[] =>
  findings.map((finding) => `${finding.Rule} (${finding.Impact}): ${finding.Help} -> ${finding.Targets.join(' | ')}`);

const ready = async (page: Page, slide = false): Promise<void> => {
  await expect(slide ? page.getByRole('dialog') : page.getByRole('heading', { level: 1 }).first()).toBeVisible();
  await page.waitForLoadState('networkidle');
};

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
  const known = list((await call('GET', `${projectBase()}/environments`, undefined, owner)).Data);
  const first = known.map((entry) => field(entry, 'Name')).find((name): name is string => typeof name === 'string');
  if (first === undefined) {
    const factor = { Method: 'password', Password: stack().Owner.Password };
    await call('POST', `${projectBase()}/environments`, { Name: ENVIRONMENT, CurrentFactor: factor }, owner);
  } else {
    environment = first;
  }
  const runs = list((await call('GET', `${projectBase()}/runs`, undefined, owner)).Data);
  const existing = runs.map((entry) => field(entry, 'Id')).find((id): id is string => typeof id === 'string');
  if (existing === undefined) {
    const branch = `feature/a11y-${Date.now().toString(36)}`;
    await push(branch);
    runId = (await runOn(owner, branch, 0)).Id;
  } else {
    runId = existing;
  }
  expect(text({ Id: runId }, 'Id')).not.toBe('');
});

const visit = async (page: Page, place: IPlace, applied: Set<TStub>): Promise<void> => {
  if (place.Stub === 'board' && !applied.has('board')) {
    await stubBoard(page, stack().OrgId, boardOf(6, 3));
    applied.add('board');
  }
  if (place.Stub === 'log' && !applied.has('log')) {
    await stubLog(page, stack().OrgId, LOG_LINES);
    applied.add('log');
  }
  await page.goto(place.Path);
  await ready(page, place.Slide === true);
  if (place.Stub === 'board') await expect(page.locator('[data-target]').first()).toBeVisible();
  if (place.Stub === 'log') await expect(page.locator('[role="log"]')).toContainText(lineText(LOG_LINES));
};

const sweep = async (browser: Browser, lang: TLang, viewport: { width: number; height: number }, theme: TThemeId, group: TGroup): Promise<void> => {
  const view = await open(browser, lang, viewport);
  await view.Context.addInitScript((value: TThemeId) => {
    window.localStorage.setItem('dhow.theme', value);
  }, theme);
  const { Page: page } = view;
  const blocking: string[] = [];
  const report: Record<string, IFinding[]> = {};
  const applied = new Set<TStub>();

  await setTheme(owner, theme);
  if (group === 'pages') {
    await page.goto('/login');
    await ready(page);
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    const login = await scan(page);
    blocking.push(...describeFindings(login.Blocking).map((line) => `login: ${line}`));
    report.login = login.Report;
  }

  await signInPage(view, stack().Owner, owner);
  for (const place of placesOf(group)) {
    await visit(page, place, applied);
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    const result = await scan(page);
    blocking.push(...describeFindings(result.Blocking).map((line) => `${place.Name} (${place.Path}): ${line}`));
    report[place.Name] = result.Report;
  }

  const label = `${group}-${lang}-${theme}-${String(viewport.width)}`;
  await test.info().attach(`axe-report-${label}.json`, { body: JSON.stringify(report, null, 2), contentType: 'application/json' });
  expect(blocking).toEqual([]);
  await expectClean(view);
  await view.Context.close();
};

for (const lang of LANGS) {
  for (const theme of THEMES) {
    for (const group of GROUPS) {
      test(`axe finds nothing serious or critical on the ${group} in ${lang} with the ${theme} theme`, async ({ browser }) => {
        test.setTimeout(SWEEP_TIMEOUT_MS);
        await sweep(browser, lang, DESKTOP, theme, group);
      });
    }
  }
}

for (const lang of LANGS) {
  for (const group of GROUPS) {
    test(`axe finds nothing serious or critical on the ${group} in ${lang} on a phone`, async ({ browser }) => {
      test.setTimeout(SWEEP_TIMEOUT_MS);
      await sweep(browser, lang, PHONE, PHONE_THEME, group);
    });
  }
}

for (const lang of LANGS) {
  test(`the keyboard reaches the core pages without a hidden focus in ${lang}`, async ({ browser }) => {
    const view = await open(browser, lang, PHONE);
    const { Page: page } = view;
    await signInPage(view, stack().Owner, owner);
    const covered: string[] = [];
    for (const place of pagePlaces(context()).filter((entry) => entry.Core === true)) {
      await page.goto(place.Path);
      await ready(page);
      covered.push(...(await focusCovered(page, 30)).map((line) => `${place.Name}: ${line}`));
    }
    expect(covered).toEqual([]);
    await view.Context.close();
  });
}

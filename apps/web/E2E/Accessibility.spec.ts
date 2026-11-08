import { expect, test, type Page } from '@playwright/test';

import { focusCovered, scan, THEMES, type IFinding, type TThemeId } from './Axe';
import { DESKTOP, expectClean, open, PHONE, signInPage, type TLang } from './Guard';
import { call, field, list, push, runOn, setTheme, signIn, stack, text, type ISession } from './Stack';

test.describe.configure({ mode: 'serial' });

const LANGS: TLang[] = ['ar', 'en'];
const ENVIRONMENT = 'a11y-check';

interface IPlace {
  Name: string;
  Path: string;
}

let owner: ISession;
let environment = ENVIRONMENT;
let runId = '';

const projectBase = () => `/organizations/${stack().OrgId}/projects/${stack().ProjectId}`;

const places = (): IPlace[] => [
  { Name: 'home', Path: '/' },
  { Name: 'projects', Path: '/projects' },
  { Name: 'project', Path: `/projects/${stack().ProjectId}` },
  { Name: 'run', Path: `/projects/${stack().ProjectId}/runs/${runId}` },
  { Name: 'environment', Path: `/projects/${stack().ProjectId}/environments/${environment}` },
  { Name: 'members', Path: '/org/members' },
  { Name: 'organization settings', Path: '/org/settings' },
  { Name: 'project settings', Path: `/projects/${stack().ProjectId}/settings` },
  { Name: 'preferences', Path: '/account/preferences' },
  { Name: 'deploy board', Path: `/projects/${stack().ProjectId}/deploy` },
  { Name: 'about', Path: '/about' },
  { Name: 'help', Path: '/help' },
];

const describeFindings = (findings: IFinding[]): string[] =>
  findings.map((finding) => `${finding.Rule} (${finding.Impact}): ${finding.Help} -> ${finding.Targets.join(' | ')}`);

const ready = async (page: Page): Promise<void> => {
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
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

for (const lang of LANGS) {
  for (const theme of THEMES) {
    test(`axe finds nothing serious or critical in ${lang} with the ${theme} theme`, async ({ browser }) => {
      const view = await open(browser, lang, DESKTOP);
      await view.Context.addInitScript((value: TThemeId) => {
        window.localStorage.setItem('dhow.theme', value);
      }, theme);
      const { Page: page } = view;
      const blocking: string[] = [];
      const report: Record<string, IFinding[]> = {};

      await setTheme(owner, theme);
      await page.goto('/login');
      await ready(page);
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      const login = await scan(page);
      blocking.push(...describeFindings(login.Blocking).map((line) => `login: ${line}`));
      report.login = login.Report;

      await signInPage(view, stack().Owner, owner);
      for (const place of places()) {
        await page.goto(place.Path);
        await ready(page);
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
        const result = await scan(page);
        blocking.push(...describeFindings(result.Blocking).map((line) => `${place.Name}: ${line}`));
        report[place.Name] = result.Report;
      }

      await test.info().attach(`axe-report-${lang}-${theme}.json`, { body: JSON.stringify(report, null, 2), contentType: 'application/json' });
      expect(blocking).toEqual([]);
      await expectClean(view);
      await view.Context.close();
    });
  }
}

for (const lang of LANGS) {
  test(`the keyboard reaches the core pages without a hidden focus in ${lang}`, async ({ browser }) => {
    const view = await open(browser, lang, PHONE);
    const { Page: page } = view;
    await signInPage(view, stack().Owner, owner);
    const covered: string[] = [];
    for (const place of places().slice(0, 5)) {
      await page.goto(place.Path);
      await ready(page);
      covered.push(...(await focusCovered(page, 30)).map((line) => `${place.Name}: ${line}`));
    }
    expect(covered).toEqual([]);
    await view.Context.close();
  });
}

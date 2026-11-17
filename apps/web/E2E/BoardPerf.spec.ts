import { expect, test } from '@playwright/test';

import { BOARD_ENVIRONMENTS, BOARD_TARGETS_PER_ENVIRONMENT, boardOf, environmentName, stubBoard } from './BoardFixture';
import { DESKTOP, expectClean, open, signInPage } from './Guard';
import {
  BOARD_FIRST_ROW_LIMITS,
  BOARD_LIMITS,
  boardFailures,
  CPU_THROTTLE,
  measure,
  observeLongTasks,
  resetLongTasks,
  scrollThrough,
  throttleCpu,
  withinLimits,
} from './Perf';
import { signIn, stack, type ISession } from './Stack';

test.describe.configure({ mode: 'serial', retries: 1 });

const ROWS = '[data-target]';

let owner: ISession;

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
});

test(`the deploy board with ${String(BOARD_ENVIRONMENTS)} environments and ${String(BOARD_ENVIRONMENTS * BOARD_TARGETS_PER_ENVIRONMENT)} targets`, async ({ browser }) => {
  const view = await open(browser, 'en', DESKTOP);
  const { Page: page } = view;
  await observeLongTasks(page);
  await signInPage(view, stack().Owner, owner);
  await stubBoard(page, stack().OrgId, boardOf(BOARD_ENVIRONMENTS, BOARD_TARGETS_PER_ENVIRONMENT));
  const session = await throttleCpu(page, CPU_THROTTLE);

  const started = Date.now();
  await page.goto(`/projects/${stack().ProjectId}/deploy`);
  await expect(page.locator(ROWS).first()).toBeVisible({ timeout: 120_000 });
  const firstRowMs = Date.now() - started;
  await expect(page.locator(ROWS)).toHaveCount(BOARD_ENVIRONMENTS * BOARD_TARGETS_PER_ENVIRONMENT);
  await expect(page.getByRole('heading', { level: 4, name: environmentName(BOARD_ENVIRONMENTS - 1) })).toHaveCount(1);
  const loaded = await measure(page, session);

  await page.waitForTimeout(1000);
  await resetLongTasks(page);
  await scrollThrough(page, 'main', 20);
  const scrolled = await measure(page, session);

  const baseline = {
    Environments: BOARD_ENVIRONMENTS,
    Targets: BOARD_ENVIRONMENTS * BOARD_TARGETS_PER_ENVIRONMENT,
    CpuThrottle: CPU_THROTTLE,
    FirstRowMs: firstRowMs,
    Loaded: loaded,
    Scrolled: scrolled,
  };
  await test.info().attach('board-baseline.json', { body: JSON.stringify(baseline, null, 2), contentType: 'application/json' });
  process.stdout.write(`perf-baseline ${JSON.stringify(baseline)}\n`);

  expect(withinLimits(loaded, BOARD_LIMITS)).toEqual([]);
  expect(withinLimits(scrolled, BOARD_LIMITS)).toEqual([]);
  expect(boardFailures(firstRowMs, scrolled, BOARD_FIRST_ROW_LIMITS)).toEqual([]);
  await expectClean(view);
  await view.Context.close();
});

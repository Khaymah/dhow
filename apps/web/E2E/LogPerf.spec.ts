import { expect, test } from '@playwright/test';

import { DESKTOP, expectClean, open, signInPage } from './Guard';
import { lineText, LOG_JOB_ID, stubLog } from './LogFixture';
import {
  CPU_THROTTLE,
  LOG_LIMITS,
  measure,
  observeLongTasks,
  resetLongTasks,
  scrollThrough,
  throttleCpu,
  VIEWER_LIMITS,
  viewerFailures,
  withinLimits,
} from './Perf';
import { signIn, stack, type ISession } from './Stack';

test.describe.configure({ mode: 'serial', retries: 1 });

const SIZES = [5_000, 50_000];
const LOG = '[role="log"]';
const ROWS = `${LOG} [data-line]`;

let owner: ISession;

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
});

for (const lines of SIZES) {
  test(`the virtualized job log page with ${lines} lines`, async ({ browser }) => {
    const view = await open(browser, 'en', DESKTOP);
    const { Page: page } = view;
    await observeLongTasks(page);
    await signInPage(view, stack().Owner, owner);
    await stubLog(page, stack().OrgId, lines);
    const session = await throttleCpu(page, CPU_THROTTLE);

    const started = Date.now();
    await page.goto(`/jobs/${LOG_JOB_ID}`);
    await expect(page.locator(LOG)).toContainText(lineText(lines), { timeout: 120_000 });
    const renderMs = Date.now() - started;

    await expect(page.locator(ROWS).first()).toBeVisible();
    const firstRowMs = Date.now() - started;
    const rendered = await page.locator(ROWS).count();
    const loaded = await measure(page, session);
    await page.waitForTimeout(1000);
    await resetLongTasks(page);
    await scrollThrough(page, LOG, 20);
    const scrolled = await measure(page, session);
    const renderedWhileScrolling = await page.locator(ROWS).count();

    const baseline = {
      Lines: lines,
      CpuThrottle: CPU_THROTTLE,
      RenderMs: renderMs,
      FirstRowMs: firstRowMs,
      RenderedRows: rendered,
      RenderedRowsWhileScrolling: renderedWhileScrolling,
      Loaded: loaded,
      Scrolled: scrolled,
    };
    await test.info().attach(`log-baseline-${lines}.json`, { body: JSON.stringify(baseline, null, 2), contentType: 'application/json' });
    process.stdout.write(`perf-baseline ${JSON.stringify(baseline)}\n`);

    expect(withinLimits(loaded, LOG_LIMITS)).toEqual([]);
    expect(withinLimits(scrolled, LOG_LIMITS)).toEqual([]);
    expect(viewerFailures(Math.max(rendered, renderedWhileScrolling), firstRowMs, scrolled, VIEWER_LIMITS)).toEqual([]);
    await expectClean(view);
    await view.Context.close();
  });
}

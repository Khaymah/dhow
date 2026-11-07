import { expect, test } from '@playwright/test';

import { DESKTOP, expectClean, open, signInPage } from './Guard';
import { lineText, LOG_JOB_ID, stubLog } from './LogFixture';
import { CPU_THROTTLE, LOOSE_LIMITS, measure, observeLongTasks, resetLongTasks, scrollThrough, throttleCpu, withinLimits } from './Perf';
import { signIn, stack, type ISession } from './Stack';

test.describe.configure({ mode: 'serial' });

const SIZES = [5_000, 50_000];
const LOG = '[role="log"]';

let owner: ISession;

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
});

for (const lines of SIZES) {
  test(`baseline of the current job log page with ${lines} lines`, async ({ browser }) => {
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

    const rendered = await page.locator(`${LOG} > *`).count();
    const loaded = await measure(page, session);
    await resetLongTasks(page);
    await scrollThrough(page, LOG, 20);
    const scrolled = await measure(page, session);

    const baseline = {
      Lines: lines,
      CpuThrottle: CPU_THROTTLE,
      RenderMs: renderMs,
      RenderedRows: rendered,
      Loaded: loaded,
      Scrolled: scrolled,
    };
    await test.info().attach(`log-baseline-${lines}.json`, { body: JSON.stringify(baseline, null, 2), contentType: 'application/json' });
    process.stdout.write(`perf-baseline ${JSON.stringify(baseline)}\n`);

    expect(withinLimits(loaded, LOOSE_LIMITS)).toEqual([]);
    expect(withinLimits(scrolled, LOOSE_LIMITS)).toEqual([]);
    expect(renderMs).toBeLessThan(120_000);
    await expectClean(view);
    await view.Context.close();
  });
}

import { expect, test } from '@playwright/test';

import { boardOf, stubBoard } from './BoardFixture';
import { scan } from './Axe';
import { DESKTOP, expectClean, open, PHONE, signInPage, type TLang } from './Guard';
import { signIn, stack, type ISession } from './Stack';

test.describe.configure({ mode: 'serial' });

const LANGS: TLang[] = ['ar', 'en'];

let owner: ISession;

test.beforeAll(async () => {
  owner = await signIn(stack().Owner);
});

for (const lang of LANGS) {
  for (const viewport of [DESKTOP, PHONE]) {
    test(`axe finds nothing serious on the deploy board with every status in ${lang} at ${String(viewport.width)}px`, async ({ browser }) => {
      const view = await open(browser, lang, viewport);
      const { Page: page } = view;
      await signInPage(view, stack().Owner, owner);
      await stubBoard(page, stack().OrgId, boardOf(40, 2));
      await page.goto(`/projects/${stack().ProjectId}/deploy`);
      await expect(page.locator('[data-target]').first()).toBeVisible();
      await expect(page.getByRole('progressbar').first()).toBeVisible();
      await expect(page.locator('[data-tone="bad"]').first()).toBeVisible();
      const result = await scan(page);
      const lines = result.Blocking.map((finding) => `${finding.Rule} (${finding.Impact}): ${finding.Help} -> ${finding.Targets.join(' | ')}`);
      await test.info().attach(`axe-board-${lang}-${String(viewport.width)}.json`, { body: JSON.stringify(result.Report, null, 2), contentType: 'application/json' });
      expect(lines).toEqual([]);
      await expectClean(view);
      await view.Context.close();
    });
  }
}

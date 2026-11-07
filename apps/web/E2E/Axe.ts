import AxeBuilder from '@axe-core/playwright';
import type { Page } from '@playwright/test';

export const BLOCKING = ['serious', 'critical'] as const;
export const THEMES = ['sea', 'teak', 'dusk', 'indigo'] as const;
export type TThemeId = (typeof THEMES)[number];

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'];

export interface IFinding {
  Rule: string;
  Impact: string;
  Help: string;
  Targets: string[];
}

export interface IScan {
  Blocking: IFinding[];
  Report: IFinding[];
}

const isBlocking = (impact: string | null | undefined): boolean => BLOCKING.some((level) => level === impact);

export const scan = async (page: Page): Promise<IScan> => {
  const result = await new AxeBuilder({ page }).withTags(TAGS).analyze();
  const findings: IFinding[] = result.violations.map((violation) => ({
    Rule: violation.id,
    Impact: violation.impact ?? 'minor',
    Help: violation.help,
    Targets: violation.nodes.slice(0, 5).map((node) => node.target.join(' ')),
  }));
  return {
    Blocking: findings.filter((finding) => isBlocking(finding.Impact)),
    Report: findings.filter((finding) => !isBlocking(finding.Impact)),
  };
};

export const focusCovered = async (page: Page, steps: number): Promise<string[]> => {
  const covered: string[] = [];
  for (let step = 0; step < steps; step += 1) {
    await page.keyboard.press('Tab');
    const hidden = await page.evaluate(() => {
      const element = document.activeElement;
      if (element === null || element === document.body) return null;
      const box = element.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) return null;
      const top = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
      if (top === null || element.contains(top) || top.contains(element)) return null;
      return `${element.tagName.toLowerCase()} ${element.getAttribute('aria-label') ?? element.textContent.trim().slice(0, 40)} is under ${top.tagName.toLowerCase()}`;
    });
    if (hidden !== null) covered.push(hidden);
  }
  return covered;
};

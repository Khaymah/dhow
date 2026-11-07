import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { chunkName, evaluate, KB, lazyBudgetKb, overrideMatches, parseOverrides, withOverrides } from './SizeBudget.mjs';
import budgets from './SizeBudgets.json' with { type: 'json' };

const sample = (lazy) => ({
  InitialJs: [['/assets/index-Ca7MMyuI.js', 100 * KB]],
  InitialCss: [['/assets/index-aX8bgqWE.css', 10 * KB]],
  Lazy: lazy,
});

describe('chunkName', () => {
  it('drops the content hash and the extension', () => {
    expect(chunkName('/assets/LogViewer-Ca7MMyuI.js')).toBe('LogViewer');
    expect(chunkName('/assets/useSlideGuardStore-cs-62bg-.js')).toBe('useSlideGuardStore');
    expect(chunkName('/assets/is-svg-component-B1Qxi7QF.js')).toBe('is-svg-component');
    expect(chunkName('/assets/dist-D322Jb2e.js')).toBe('dist');
    expect(chunkName('/assets/index-Ca7MMyuI.js')).toBe('index');
  });

  it('leaves a name without a hash alone', () => {
    expect(chunkName('/assets/LogViewer.js')).toBe('LogViewer');
    expect(chunkName('/assets/is-svg-component.js')).toBe('is-svg-component');
  });

  it('lets the log viewer pattern match its hashed chunk and nothing else', () => {
    const names = ['/assets/LogViewer-D322Jb2e.js', '/assets/JobPage-B471HyZt.js', '/assets/LogViewerToolbar-D322Jb2e.js'].map(chunkName);
    const [rule] = overrideMatches(names, budgets);
    expect(rule?.Pattern).toBe('^LogViewer');
    expect(rule?.Matches).toBe(2);
    expect(overrideMatches(['JobPage'], budgets)[0]?.Matches).toBe(0);
  });
});

describe('lazyBudgetKb', () => {
  it('gives 60 KB to a normal chunk and 35 KB to the log viewer', () => {
    expect(lazyBudgetKb('JobPage', budgets)).toBe(60);
    expect(lazyBudgetKb('LogViewer', budgets)).toBe(35);
  });
});

describe('evaluate', () => {
  it('passes when every chunk is inside its budget', () => {
    const report = evaluate(sample([['/assets/JobPage-Ca7MMyuI.js', 8 * KB], ['/assets/LogViewer-Ca7MMyuI.js', 34 * KB]]), budgets);
    expect(report.Failed).toBe(false);
  });

  it('fails a lazy chunk over 60 KB', () => {
    const report = evaluate(sample([['/assets/JobPage-Ca7MMyuI.js', 61 * KB]]), budgets);
    expect(report.Failed).toBe(true);
    expect(report.Lazy[0]?.Over).toBe(true);
  });

  it('fails the log viewer chunk over 35 KB although it is under the default', () => {
    const report = evaluate(sample([['/assets/LogViewer-Ca7MMyuI.js', 40 * KB]]), budgets);
    expect(report.Failed).toBe(true);
  });

  it('fails when the initial js is over its budget', () => {
    const report = evaluate({ ...sample([]), InitialJs: [['/assets/index-Ca7MMyuI.js', 129 * KB]] }, budgets);
    expect(report.Failed).toBe(true);
  });

  it('fails with an artificially lowered lazy budget', () => {
    const lowered = withOverrides(budgets, { LazyJsKb: 5 });
    const report = evaluate(sample([['/assets/JobPage-Ca7MMyuI.js', 8 * KB]]), lowered);
    expect(report.Failed).toBe(true);
  });
});

describe('parseOverrides', () => {
  it('reads the budget flags and rejects a bad number', () => {
    expect(parseOverrides(['--lazy-kb=5']).LazyJsKb).toBe(5);
    expect(parseOverrides([]).LazyJsKb).toBeUndefined();
    expect(() => parseOverrides(['--lazy-kb=zero'])).toThrow();
  });

  it('refuses an override that would raise a budget', () => {
    expect(() => withOverrides(budgets, { LazyJsKb: 61 })).toThrow(/can only lower/);
    expect(() => withOverrides(budgets, { InitialJsKb: 200 })).toThrow(/can only lower/);
    expect(() => withOverrides(budgets, { InitialCssKb: 31 })).toThrow(/can only lower/);
    expect(withOverrides(budgets, { LazyJsKb: 60 }).LazyJsKb).toBe(60);
  });
});

const dist = join(process.cwd(), 'dist', 'index.html');
const script = join(process.cwd(), 'Scripts', 'CheckSize.mjs');

describe.skipIf(!existsSync(dist))('pnpm size on a real build', () => {
  it('passes with the committed budgets and fails with a lowered lazy budget', () => {
    expect(spawnSync('node', [script], { encoding: 'utf8' }).status).toBe(0);
    const lowered = spawnSync('node', [script, '--lazy-kb=1'], { encoding: 'utf8' });
    expect(lowered.status).toBe(1);
    expect(lowered.stdout).toContain('OVER');
    expect(lowered.stdout).toContain('lazy override ^LogViewer');
    const raised = spawnSync('node', [script, '--lazy-kb=500'], { encoding: 'utf8' });
    expect(raised.status).toBe(1);
    expect(raised.stderr).toContain('can only lower');
  });
});

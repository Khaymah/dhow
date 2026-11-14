import type { CDPSession, Page } from '@playwright/test';

export interface ILongTaskStats {
  Count: number;
  TotalMs: number;
  MaxMs: number;
}

export interface IPerfSample {
  DomNodes: number;
  LongTasks: ILongTaskStats;
  HeapUsedBytes: number;
}

export interface IPerfLimits {
  MaxDomNodes: number;
  MaxLongTaskMs: number;
  MaxHeapBytes: number;
}

export const LOG_LIMITS: IPerfLimits = {
  MaxDomNodes: 3_000,
  MaxLongTaskMs: 1_000,
  MaxHeapBytes: 100_000_000,
};

export const BOARD_LIMITS: IPerfLimits = {
  MaxDomNodes: 20_000,
  MaxLongTaskMs: 2_000,
  MaxHeapBytes: 100_000_000,
};

export interface IViewerLimits {
  MaxRows: number;
  MaxScrollLongTaskMs: number;
  MaxFirstRowMs: number;
}

export const VIEWER_LIMITS: IViewerLimits = {
  MaxRows: 300,
  MaxScrollLongTaskMs: 100,
  MaxFirstRowMs: 4_000,
};

export interface IBoardLimits {
  MaxFirstRowMs: number;
  MaxScrollLongTaskMs: number;
}

export const BOARD_FIRST_ROW_LIMITS: IBoardLimits = {
  MaxFirstRowMs: 5_000,
  MaxScrollLongTaskMs: 100,
};

export const CPU_THROTTLE = 4;
const TASKS_KEY = '__dhowLongTasks';

interface ILongTaskEntry {
  duration: number;
}

export const observeLongTasks = async (page: Page): Promise<void> => {
  await page.addInitScript((key) => {
    const tasks: ILongTaskEntry[] = [];
    Object.defineProperty(window, key, { value: tasks, configurable: true });
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) tasks.push({ duration: entry.duration });
      }).observe({ type: 'longtask', buffered: true });
    } catch {
      return;
    }
  }, TASKS_KEY);
};

export const throttleCpu = async (page: Page, rate: number = CPU_THROTTLE): Promise<CDPSession> => {
  const session = await page.context().newCDPSession(page);
  await session.send('Emulation.setCPUThrottlingRate', { rate });
  return session;
};

export const resetLongTasks = async (page: Page): Promise<void> => {
  await page.evaluate((key) => {
    const tasks = Reflect.get(window, key) as ILongTaskEntry[];
    tasks.length = 0;
  }, TASKS_KEY);
};

const longTasks = async (page: Page): Promise<ILongTaskStats> => {
  const durations = await page.evaluate((key) => (Reflect.get(window, key) as ILongTaskEntry[]).map((entry) => entry.duration), TASKS_KEY);
  return {
    Count: durations.length,
    TotalMs: Math.round(durations.reduce((sum, value) => sum + value, 0)),
    MaxMs: Math.round(durations.reduce((max, value) => Math.max(max, value), 0)),
  };
};

const heapUsed = async (session: CDPSession): Promise<number> => {
  await session.send('HeapProfiler.collectGarbage');
  const { metrics } = await session.send('Performance.getMetrics');
  return metrics.find((metric) => metric.name === 'JSHeapUsedSize')?.value ?? 0;
};

export const measure = async (page: Page, session: CDPSession): Promise<IPerfSample> => {
  await session.send('Performance.enable');
  const domNodes = await page.evaluate(() => document.querySelectorAll('*').length);
  return { DomNodes: domNodes, LongTasks: await longTasks(page), HeapUsedBytes: await heapUsed(session) };
};

export const withinLimits = (sample: IPerfSample, limits: IPerfLimits): string[] => {
  const failures: string[] = [];
  if (sample.DomNodes > limits.MaxDomNodes) failures.push(`DOM nodes ${sample.DomNodes} > ${limits.MaxDomNodes}`);
  if (sample.LongTasks.MaxMs > limits.MaxLongTaskMs) failures.push(`longest task ${sample.LongTasks.MaxMs}ms > ${limits.MaxLongTaskMs}ms`);
  if (sample.HeapUsedBytes > limits.MaxHeapBytes) failures.push(`heap ${sample.HeapUsedBytes} > ${limits.MaxHeapBytes}`);
  return failures;
};

export const viewerFailures = (rows: number, firstRowMs: number, scrolled: IPerfSample, limits: IViewerLimits): string[] => {
  const failures: string[] = [];
  if (firstRowMs > limits.MaxFirstRowMs) failures.push(`first log row after ${firstRowMs}ms > ${limits.MaxFirstRowMs}ms`);
  if (rows >= limits.MaxRows) failures.push(`log rows in the DOM ${rows} >= ${limits.MaxRows}`);
  if (scrolled.LongTasks.MaxMs > limits.MaxScrollLongTaskMs) {
    failures.push(`longest task while scrolling ${scrolled.LongTasks.MaxMs}ms > ${limits.MaxScrollLongTaskMs}ms`);
  }
  return failures;
};

export const boardFailures = (firstRowMs: number, scrolled: IPerfSample, limits: IBoardLimits): string[] => {
  const failures: string[] = [];
  if (firstRowMs > limits.MaxFirstRowMs) failures.push(`first board row after ${firstRowMs}ms > ${limits.MaxFirstRowMs}ms`);
  if (scrolled.LongTasks.MaxMs > limits.MaxScrollLongTaskMs) {
    failures.push(`longest task while scrolling ${scrolled.LongTasks.MaxMs}ms > ${limits.MaxScrollLongTaskMs}ms`);
  }
  return failures;
};

export const scrollThrough = async (page: Page, selector: string, steps: number): Promise<void> => {
  for (let step = 1; step <= steps; step += 1) {
    await page.evaluate(
      ({ target, fraction }) => {
        const element = document.querySelector(target);
        if (element === null) throw new Error(`no element for ${target}`);
        element.scrollTop = (element.scrollHeight - element.clientHeight) * fraction;
      },
      { target: selector, fraction: 1 - step / steps },
    );
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => { resolve(); })));
  }
};

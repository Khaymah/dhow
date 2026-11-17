import { describe, expect, it } from 'vitest';

import { BOARD_FIRST_ROW_LIMITS, BOARD_LIMITS, boardFailures, LOG_LIMITS, VIEWER_LIMITS, viewerFailures, withinLimits, type IPerfSample } from './Perf';

const sample = (overrides: Partial<IPerfSample> = {}): IPerfSample => ({
  DomNodes: 704,
  LongTasks: { Count: 0, TotalMs: 0, MaxMs: 0 },
  HeapUsedBytes: 19_000_000,
  ...overrides,
});

describe('the performance limits', () => {
  it('accept the CI baseline of the log viewer with 50,000 lines', () => {
    const loaded = sample({ LongTasks: { Count: 6, TotalMs: 643, MaxMs: 249 } });
    expect(withinLimits(loaded, LOG_LIMITS)).toEqual([]);
    expect(viewerFailures(36, 2_483, sample(), VIEWER_LIMITS)).toEqual([]);
  });

  it('accept the CI baseline of the deploy board with 400 targets', () => {
    const loaded = sample({ DomNodes: 12_210, HeapUsedBytes: 14_100_000, LongTasks: { Count: 7, TotalMs: 1_746, MaxMs: 795 } });
    expect(withinLimits(loaded, BOARD_LIMITS)).toEqual([]);
    expect(boardFailures(2_967, sample({ LongTasks: { Count: 1, TotalMs: 57, MaxMs: 57 } }), BOARD_FIRST_ROW_LIMITS)).toEqual([]);
  });

  it('name every limit a sample breaks', () => {
    const broken = sample({ DomNodes: 5_000, HeapUsedBytes: 200_000_000, LongTasks: { Count: 1, TotalMs: 2_000, MaxMs: 2_000 } });
    expect(withinLimits(broken, LOG_LIMITS)).toHaveLength(3);
  });

  it('fail a viewer that renders every row, opens late or janks while scrolling', () => {
    const janky = sample({ LongTasks: { Count: 1, TotalMs: 119, MaxMs: 119 } });
    expect(viewerFailures(300, 4_001, janky, VIEWER_LIMITS)).toHaveLength(3);
  });

  it('fail a board that opens late or janks while scrolling', () => {
    const janky = sample({ LongTasks: { Count: 1, TotalMs: 101, MaxMs: 101 } });
    expect(boardFailures(5_001, janky, BOARD_FIRST_ROW_LIMITS)).toHaveLength(2);
  });
});

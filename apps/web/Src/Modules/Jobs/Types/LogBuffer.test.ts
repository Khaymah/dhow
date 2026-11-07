import { describe, expect, it } from 'vitest';

import type { TJobLogChunk } from '@/Modules/Jobs/Api/JobsApi';
import {
  appendChunks,
  EMPTY_LOG,
  hasOlder,
  MAX_LINE_LENGTH,
  MAX_LOG_LINES,
  prependChunks,
  systemCodeOf,
  type ILogBuffer,
} from '@/Modules/Jobs/Types/LogBuffer';

const chunk = (seq: number, text: string, extra: Partial<TJobLogChunk> = {}): TJobLogChunk => ({
  Seq: seq,
  Stream: 'stdout',
  StepIndex: 0,
  CreatedAt: '2026-10-28T10:00:00Z',
  Text: text,
  ...extra,
});

const texts = (buffer: ILogBuffer) =>
  buffer.Lines.filter((line) => line.Kind !== 'step').map((line) => line.Segments.map((segment) => segment.Text).join(''));

describe('appendChunks', () => {
  it('starts each step with a header and splits lines across chunks', () => {
    const buffer = appendChunks(EMPTY_LOG, [chunk(1, 'a\nb'), chunk(2, 'c\n'), chunk(3, 'd\n', { StepIndex: 1 })]);
    expect(buffer.Lines.map((line) => line.Kind)).toEqual(['step', 'text', 'text', 'step', 'text']);
    expect(texts(buffer)).toEqual(['a', 'bc', 'd']);
    expect(buffer.LastSeq).toBe(3);
  });

  it('ignores chunks it already has, so a resumed stream never repeats a line', () => {
    const once = appendChunks(EMPTY_LOG, [chunk(1, 'a\n'), chunk(2, 'b\n')]);
    const again = appendChunks(once, [chunk(2, 'b\n'), chunk(3, 'c\n')]);
    expect(texts(again)).toEqual(['a', 'b', 'c']);
    expect(appendChunks(again, [chunk(1, 'a\n')])).toBe(again);
  });

  it('keeps stdout and stderr on separate lines and server lines on their own', () => {
    const buffer = appendChunks(EMPTY_LOG, [
      chunk(1, 'out'),
      chunk(2, 'err\n', { Stream: 'stderr' }),
      chunk(3, 'dhow: LOG_TRUNCATED: the log reached its limit', { Stream: 'system', StepIndex: undefined }),
    ]);
    expect(buffer.Lines.slice(1).map((line) => [line.Kind, line.Stream])).toEqual([
      ['text', 'stdout'],
      ['text', 'stderr'],
      ['system', 'system'],
    ]);
    expect(systemCodeOf(buffer.Lines.at(-1)?.Raw ?? '')).toBe('LOG_TRUNCATED');
  });

  it('shows only what follows the last carriage return, like a terminal', () => {
    const buffer = appendChunks(EMPTY_LOG, [chunk(1, '10%\r50%\r100%\r\n')]);
    expect(texts(buffer)).toEqual(['100%']);
  });

  it('keeps the last lines only and counts the ones it dropped', () => {
    const text = Array.from({ length: MAX_LOG_LINES + 50 }, (_, index) => `line ${String(index)}\n`).join('');
    const buffer = appendChunks(EMPTY_LOG, [chunk(1, text)]);
    expect(buffer.Lines).toHaveLength(MAX_LOG_LINES);
    expect(buffer.Dropped).toBe(51);
    expect(texts(buffer).at(-1)).toBe(`line ${String(MAX_LOG_LINES + 49)}`);
  });

  it('breaks a very long line into pieces', () => {
    const buffer = appendChunks(EMPTY_LOG, [chunk(1, 'x'.repeat(MAX_LINE_LENGTH * 2 + 5))]);
    expect(texts(buffer).map((line) => line.length)).toEqual([MAX_LINE_LENGTH, MAX_LINE_LENGTH, 5]);
  });
});

const numbered = (from: number, count: number, extra: Partial<TJobLogChunk> = {}): TJobLogChunk[] =>
  Array.from({ length: count }, (_, index) => chunk(from + index, `line ${String(from + index)}\n`, extra));

describe('the line cap and older lines', () => {
  it('keeps one hundred thousand lines', () => {
    expect(MAX_LOG_LINES).toBe(100_000);
  });

  it('remembers the chunk each line started in', () => {
    const buffer = appendChunks(EMPTY_LOG, [chunk(4, 'a\nb'), chunk(5, 'c\nd\n')]);
    expect(buffer.Lines.map((line) => [line.Kind, line.Seq])).toEqual([
      ['step', 4],
      ['text', 4],
      ['text', 4],
      ['text', 5],
    ]);
  });

  it('cuts at a chunk boundary so older lines can be fetched by Seq', () => {
    const buffer = appendChunks(EMPTY_LOG, numbered(1, MAX_LOG_LINES + 10));
    expect(buffer.Lines.length).toBeLessThanOrEqual(MAX_LOG_LINES);
    expect(buffer.Lines.length).toBeGreaterThan(MAX_LOG_LINES - 5);
    expect(hasOlder(buffer)).toBe(true);
    const first = buffer.Lines[0];
    expect(first?.Seq).toBeGreaterThan(1);
    expect(buffer.Lines.at(-1)?.Seq).toBe(MAX_LOG_LINES + 10);
    expect(buffer.Dropped + buffer.Lines.length).toBe(MAX_LOG_LINES + 11);
  });

  it('has nothing older while the first chunk is still there', () => {
    expect(hasOlder(EMPTY_LOG)).toBe(false);
    expect(hasOlder(appendChunks(EMPTY_LOG, numbered(1, 10)))).toBe(false);
  });

  it('puts older lines in front with the numbers just before the first one', () => {
    const full = appendChunks(EMPTY_LOG, numbered(1, 30));
    const tail = { ...full, Lines: full.Lines.slice(21), Dropped: 21 };
    expect(tail.Lines[0]?.Seq).toBe(21);
    const older = prependChunks(tail, { Chunks: numbered(1, 30), ContextSeq: null });
    expect(older.Lines.map((line) => line.Id)).toEqual(full.Lines.map((line) => line.Id));
    expect(older.Lines.map((line) => line.Raw)).toEqual(full.Lines.map((line) => line.Raw));
    expect(older.Dropped).toBe(0);
    expect(hasOlder(older)).toBe(false);
  });

  it('uses the context chunk to know the step and then drops its lines', () => {
    const full = appendChunks(EMPTY_LOG, numbered(1, 10));
    const tail = { ...full, Lines: full.Lines.slice(6), Dropped: 6 };
    expect(tail.Lines[0]?.Seq).toBe(6);
    const older = prependChunks(tail, { Chunks: numbered(3, 3), ContextSeq: 3 });
    expect(older.Lines.slice(0, 2).map((line) => [line.Id, line.Kind, line.Raw])).toEqual([
      [5, 'text', 'line 4'],
      [6, 'text', 'line 5'],
    ]);
    expect(older.Lines).toHaveLength(tail.Lines.length + 2);
  });

  it('ignores chunks that are not older than the first line', () => {
    const base = appendChunks(EMPTY_LOG, numbered(5, 3));
    expect(prependChunks(base, { Chunks: numbered(5, 3), ContextSeq: null })).toBe(base);
    expect(prependChunks(EMPTY_LOG, { Chunks: numbered(1, 3), ContextSeq: null })).toBe(EMPTY_LOG);
  });
});

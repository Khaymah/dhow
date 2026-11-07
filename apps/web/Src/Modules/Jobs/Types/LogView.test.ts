import { describe, expect, it } from 'vitest';

import type { TJobLogChunk } from '@/Modules/Jobs/Api/JobsApi';
import { appendChunks, EMPTY_LOG } from '@/Modules/Jobs/Types/LogBuffer';
import { lineUrl, parsePositive, readAttempt, readLine } from '@/Modules/Jobs/Types/LogLink';
import { indexOfId, lowerBound, stepsOf, visibleIndexes } from '@/Modules/Jobs/Types/LogRows';
import { findMatches, nextPosition } from '@/Modules/Jobs/Types/LogSearch';

const chunk = (seq: number, text: string, step = 0): TJobLogChunk => ({
  Seq: seq,
  Stream: 'stdout',
  StepIndex: step,
  CreatedAt: '2026-11-07T10:00:00Z',
  Text: text,
});

const buffer = appendChunks(EMPTY_LOG, [chunk(1, 'Alpha\nbeta\n'), chunk(2, 'ALPHA again\n', 1), chunk(3, '\u001b[31mred alpha\u001b[0m\n', 1)]);

describe('findMatches', () => {
  it('finds lines ignoring case, by position in the buffer, and skips step headers', () => {
    const found = findMatches(buffer.Lines, 'alpha', false);
    expect(found.map((index) => buffer.Lines[index]?.Raw)).toEqual(['Alpha', 'ALPHA again', '\u001b[31mred alpha\u001b[0m']);
  });

  it('matches the case when asked and searches the text the reader sees', () => {
    expect(findMatches(buffer.Lines, 'alpha', true)).toHaveLength(1);
    expect(findMatches(buffer.Lines, '31m', false)).toEqual([]);
    expect(findMatches(buffer.Lines, '', false)).toEqual([]);
  });

  it('finds Arabic text', () => {
    const arabic = appendChunks(EMPTY_LOG, [chunk(1, 'تم النشر بنجاح\n')]);
    expect(findMatches(arabic.Lines, 'النشر', false)).toHaveLength(1);
  });
});

describe('nextPosition', () => {
  it.each([
    [0, 3, 1, 1],
    [2, 3, 1, 0],
    [0, 3, -1, 2],
    [0, 0, 1, -1],
  ] as const)('from %i of %i going %i lands on %i', (position, total, step, expected) => {
    expect(nextPosition(position, total, step)).toBe(expected);
  });
});

describe('visibleIndexes', () => {
  it('hides the lines of a collapsed step but keeps its header', () => {
    expect(visibleIndexes(buffer.Lines, new Set()).length).toBe(buffer.Lines.length);
    const shown = visibleIndexes(buffer.Lines, new Set([0]));
    expect(shown.map((index) => buffer.Lines[index]?.Kind)).toEqual(['step', 'step', 'text', 'text']);
  });

  it('lists the steps that have a header', () => {
    expect(stepsOf(buffer.Lines)).toEqual([0, 1]);
  });
});

describe('lowerBound and indexOfId', () => {
  it('finds the first position that is not below the target', () => {
    expect(lowerBound([1, 3, 5, 9], 5)).toBe(2);
    expect(lowerBound([1, 3, 5, 9], 4)).toBe(2);
    expect(lowerBound([1, 3, 5, 9], 10)).toBe(4);
    expect(lowerBound([], 1)).toBe(0);
  });

  it('finds a line by its number', () => {
    expect(indexOfId(buffer.Lines, buffer.Lines[3]?.Id ?? 0)).toBe(3);
    expect(indexOfId(buffer.Lines, 999)).toBe(-1);
    expect(indexOfId([], 1)).toBe(-1);
  });
});

describe('line links', () => {
  it.each([
    ['?line=42', 42],
    ['?line=0', null],
    ['?line=-3', null],
    ['?line=4.5', null],
    ['?line=abc', null],
    ['?line=12345678901', null],
    ['?line=007', null],
    ['', null],
  ])('reads %s as %s', (search, expected) => {
    expect(readLine(search)).toBe(expected);
  });

  it('reads the attempt the same way', () => {
    expect(readAttempt('?attempt=2&line=5')).toBe(2);
    expect(readAttempt('?line=5')).toBeNull();
    expect(parsePositive(null)).toBeNull();
  });

  it('builds a link that keeps the path and other parameters and drops the hash', () => {
    const url = lineUrl('https://dhow.example/jobs/abc?tab=x#top', 7, 2);
    expect(url).toBe('https://dhow.example/jobs/abc?tab=x&attempt=2&line=7');
  });
});

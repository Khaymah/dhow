import type { ILogLine } from '@/Modules/Jobs/Types/LogBuffer';

export const visibleIndexes = (lines: readonly ILogLine[], collapsed: ReadonlySet<number>): number[] => {
  const shown: number[] = [];
  lines.forEach((line, index) => {
    const hidden = collapsed.size > 0 && line.Kind !== 'step' && line.Step !== undefined && collapsed.has(line.Step);
    if (!hidden) shown.push(index);
  });
  return shown;
};

export const lowerBound = (sorted: readonly number[], target: number): number => {
  let low = 0;
  let high = sorted.length;
  while (low < high) {
    const middle = (low + high) >>> 1;
    if ((sorted[middle] ?? 0) < target) low = middle + 1;
    else high = middle;
  }
  return low;
};

export const stepsOf = (lines: readonly ILogLine[]): number[] => {
  const steps = new Set<number>();
  for (const line of lines) if (line.Kind === 'step' && line.Step !== undefined) steps.add(line.Step);
  return [...steps];
};

export const indexOfId = (lines: readonly ILogLine[], id: number): number => {
  const first = lines[0];
  if (first === undefined) return -1;
  const guess = id - first.Id;
  if (guess >= 0 && lines[guess]?.Id === id) return guess;
  return lines.findIndex((line) => line.Id === id);
};

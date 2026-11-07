import type { ILogLine } from '@/Modules/Jobs/Types/LogBuffer';

export const plainOf = (line: ILogLine): string => {
  let text = '';
  for (const segment of line.Segments) text += segment.Text;
  return text;
};

export const findMatches = (lines: readonly ILogLine[], query: string, matchCase: boolean): number[] => {
  if (query === '') return [];
  const needle = matchCase ? query : query.toLowerCase();
  const found: number[] = [];
  lines.forEach((line, index) => {
    if (line.Kind === 'step') return;
    const text = plainOf(line);
    if ((matchCase ? text : text.toLowerCase()).includes(needle)) found.push(index);
  });
  return found;
};

export const nextPosition = (position: number, total: number, step: 1 | -1): number => {
  if (total === 0) return -1;
  return (position + step + total) % total;
};

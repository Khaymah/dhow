import { useCallback, useMemo, useState } from 'react';

import type { ILogLine } from '@/Modules/Jobs/Types/LogBuffer';
import { findMatches, nextPosition } from '@/Modules/Jobs/Types/LogSearch';

export interface ILogSearch {
  Query: string;
  MatchCase: boolean;
  Matches: readonly number[];
  Active: number;
  ActiveLine: ILogLine | null;
  SetQuery: (query: string) => void;
  ToggleCase: () => void;
  Step: (direction: 1 | -1) => void;
  Clear: () => void;
}

export const useLogSearch = (lines: readonly ILogLine[], focusLine: (id: number) => void): ILogSearch => {
  const [Query, setQuery] = useState('');
  const [MatchCase, setMatchCase] = useState(false);
  const [Position, setPosition] = useState(0);
  const Matches = useMemo(() => findMatches(lines, Query, MatchCase), [lines, Query, MatchCase]);
  const Active = Matches.length === 0 ? -1 : Math.min(Position, Matches.length - 1);
  const ActiveLine = Active < 0 ? null : (lines[Matches[Active] ?? -1] ?? null);

  const restart = useCallback(
    (query: string, matchCase: boolean) => {
      setQuery(query);
      setMatchCase(matchCase);
      setPosition(0);
      const first = findMatches(lines, query, matchCase)[0];
      const line = first === undefined ? undefined : lines[first];
      if (line !== undefined) focusLine(line.Id);
    },
    [lines, focusLine],
  );

  const SetQuery = useCallback(
    (query: string) => {
      restart(query, MatchCase);
    },
    [restart, MatchCase],
  );

  const ToggleCase = useCallback(() => {
    restart(Query, !MatchCase);
  }, [restart, Query, MatchCase]);

  const Step = useCallback(
    (direction: 1 | -1) => {
      const position = nextPosition(Active, Matches.length, direction);
      const line = position < 0 ? undefined : lines[Matches[position] ?? -1];
      setPosition(Math.max(position, 0));
      if (line !== undefined) focusLine(line.Id);
    },
    [Active, Matches, lines, focusLine],
  );

  const Clear = useCallback(() => {
    setQuery('');
    setPosition(0);
  }, []);

  return { Query, MatchCase, Matches, Active, ActiveLine, SetQuery, ToggleCase, Step, Clear };
};

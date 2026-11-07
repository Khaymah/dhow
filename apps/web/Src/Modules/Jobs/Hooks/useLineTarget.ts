import { useCallback, useEffect, useRef, useState } from 'react';

import type { ILogLine } from '@/Modules/Jobs/Types/LogBuffer';
import { readAttempt, readLine, writeLine } from '@/Modules/Jobs/Types/LogLink';
import { indexOfId, lowerBound } from '@/Modules/Jobs/Types/LogRows';

export interface ILineTargetInput {
  lines: readonly ILogLine[];
  shown: readonly number[];
  attempt: number;
  done: boolean;
  hasOlder: boolean;
  loadingOlder: boolean;
  olderFailed: boolean;
  loadOlder: () => Promise<void>;
  expandStep: (step: number) => void;
  scrollTo: (position: number) => void;
}

export interface ILineTarget {
  Selected: number | null;
  Missing: number | null;
  Goto: (id: number) => void;
  Select: (id: number) => void;
}

const linkedLine = (attempt: number): number | null => {
  const requested = readAttempt(window.location.search);
  return requested !== null && requested !== attempt ? null : readLine(window.location.search);
};

export const useLineTarget = (input: ILineTargetInput): ILineTarget => {
  const { lines, shown, attempt, done, hasOlder, loadingOlder, olderFailed, loadOlder, expandStep, scrollTo } = input;
  const [Linked] = useState(() => linkedLine(attempt));
  const [Selected, setSelected] = useState<number | null>(Linked);
  const [Tick, setTick] = useState(0);
  const wanted = useRef<number | null>(Linked);
  const head = lines[0]?.Id;

  useEffect(() => {
    const id = wanted.current;
    if (id === null) return;
    const index = indexOfId(lines, id);
    if (index < 0) {
      if (head !== undefined && id < head && hasOlder && !loadingOlder && !olderFailed) void loadOlder();
      return;
    }
    const position = lowerBound(shown, index);
    const step = lines[index]?.Step;
    if (shown[position] !== index) {
      if (step !== undefined) expandStep(step);
      return;
    }
    wanted.current = null;
    scrollTo(position);
  }, [Tick, lines, shown, head, hasOlder, loadingOlder, olderFailed, loadOlder, expandStep, scrollTo]);

  const Goto = useCallback((id: number) => {
    wanted.current = id;
    setTick((value) => value + 1);
  }, []);

  const Select = useCallback(
    (id: number) => {
      setSelected(id);
      writeLine(id, attempt);
    },
    [attempt],
  );

  const missing = Linked !== null && done && !hasOlder && indexOfId(lines, Linked) < 0;
  return { Selected, Missing: missing ? Linked : null, Goto, Select };
};

import { useCallback, useMemo, useState } from 'react';

import type { ILogLine } from '@/Modules/Jobs/Types/LogBuffer';
import { stepsOf } from '@/Modules/Jobs/Types/LogRows';

export interface ICollapsedSteps {
  Collapsed: ReadonlySet<number>;
  Steps: readonly number[];
  AllCollapsed: boolean;
  ToggleStep: (step: number) => void;
  ExpandStep: (step: number) => void;
  ToggleAll: () => void;
}

export const useCollapsedSteps = (lines: readonly ILogLine[]): ICollapsedSteps => {
  const [Collapsed, setCollapsed] = useState<ReadonlySet<number>>(() => new Set());
  const Steps = useMemo(() => stepsOf(lines), [lines]);

  const ToggleStep = useCallback((step: number) => {
    setCollapsed((current) => {
      const next = new Set(current);
      if (!next.delete(step)) next.add(step);
      return next;
    });
  }, []);

  const ExpandStep = useCallback((step: number) => {
    setCollapsed((current) => {
      if (!current.has(step)) return current;
      const next = new Set(current);
      next.delete(step);
      return next;
    });
  }, []);

  const AllCollapsed = Steps.length > 0 && Steps.every((step) => Collapsed.has(step));
  const ToggleAll = useCallback(() => {
    setCollapsed(AllCollapsed ? new Set() : new Set(Steps));
  }, [AllCollapsed, Steps]);

  return { Collapsed, Steps, AllCollapsed, ToggleStep, ExpandStep, ToggleAll };
};

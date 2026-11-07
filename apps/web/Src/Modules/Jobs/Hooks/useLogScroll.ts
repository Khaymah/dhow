import { useVirtualizer, type Virtualizer } from '@tanstack/react-virtual';
import { useCallback, useLayoutEffect, useRef, useState, type RefObject } from 'react';

import type { ILogLine } from '@/Modules/Jobs/Types/LogBuffer';
import { indexOfId, lowerBound } from '@/Modules/Jobs/Types/LogRows';

export const ROW_ESTIMATE_PX = 22;
export const ROW_OVERSCAN = 10;
export const BOTTOM_SLACK_PX = 32;

export interface ILogScroll {
  Virtualizer: Virtualizer<HTMLDivElement, Element>;
  Ref: RefObject<HTMLDivElement | null>;
  Following: boolean;
  HandleScroll: () => void;
  JumpToEnd: () => void;
  ScrollTo: (position: number) => void;
}

const atBottom = (element: HTMLElement): boolean =>
  element.scrollHeight - element.scrollTop - element.clientHeight <= BOTTOM_SLACK_PX;

export const useLogScroll = (lines: readonly ILogLine[], shown: readonly number[]): ILogScroll => {
  const Ref = useRef<HTMLDivElement>(null);
  const headRef = useRef<number | null>(null);
  const [Following, setFollowing] = useState(true);
  // eslint-disable-next-line react-hooks/incompatible-library
  const Virtualizer = useVirtualizer<HTMLDivElement, Element>({
    count: shown.length,
    getScrollElement: () => Ref.current,
    estimateSize: () => ROW_ESTIMATE_PX,
    overscan: ROW_OVERSCAN,
    getItemKey: (index) => lines[shown[index] ?? -1]?.Id ?? index,
  });
  const total = Virtualizer.getTotalSize();

  const HandleScroll = useCallback(() => {
    const element = Ref.current;
    if (element !== null) setFollowing(atBottom(element));
  }, []);

  const JumpToEnd = useCallback(() => {
    const element = Ref.current;
    if (element !== null) element.scrollTop = element.scrollHeight;
    setFollowing(true);
  }, []);

  const ScrollTo = useCallback(
    (position: number) => {
      setFollowing(false);
      Virtualizer.scrollToIndex(position, { align: 'center' });
    },
    [Virtualizer],
  );

  useLayoutEffect(() => {
    const element = Ref.current;
    if (Following && element !== null) element.scrollTop = element.scrollHeight;
  }, [Following, total]);

  useLayoutEffect(() => {
    const head = lines[0]?.Id ?? null;
    const previous = headRef.current;
    headRef.current = head;
    if (previous === null || head === null || head >= previous) return;
    const index = indexOfId(lines, previous);
    if (index >= 0) Virtualizer.scrollToIndex(lowerBound(shown, index), { align: 'start' });
  }, [lines, shown, Virtualizer]);

  return { Virtualizer, Ref, Following, HandleScroll, JumpToEnd, ScrollTo };
};

import { useLocation } from '@tanstack/react-router';
import { useEffect, useRef, type RefObject } from 'react';

export const useAnchorTarget = (anchor: string | undefined): RefObject<HTMLDivElement | null> => {
  const hash = useLocation({ select: (location) => location.hash });
  const ref = useRef<HTMLDivElement>(null);
  const wanted = anchor !== undefined && hash.replace(/^#/, '') === anchor;

  useEffect(() => {
    if (!wanted) return;
    const element = ref.current;
    if (element === null) return;
    element.scrollIntoView();
    element.focus({ preventScroll: true });
  }, [wanted]);

  return ref;
};

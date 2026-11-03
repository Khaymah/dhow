import * as Dialog from '@radix-ui/react-dialog';
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';

import { useEnterFlag } from '@/Core/Hooks/useEnterFlag';
import { usePrefersReducedMotion } from '@/Core/Hooks/usePrefersReducedMotion';
import { wrapTab } from '@/Core/Hooks/useTabWrap';
import { SlideView, type ISlide } from '@/Core/Ui/Slide';

export type { ISlide } from '@/Core/Ui/Slide';

export type TJumpSource = 'crumb' | 'slide';

export interface ISlideCloseGuard {
  blocked: boolean;
  onBlocked: () => void;
}

export interface ISlideStackProps {
  slides: readonly ISlide[];
  onPop: () => void;
  onJump: (index: number, source: TJumpSource) => void;
  onCloseAll: () => void;
  closeGuard?: ISlideCloseGuard;
}

interface IGhost {
  slide: ISlide;
  index: number;
  total: number;
  trail: string[];
}

const EXIT_MS = 450;

const toGhost = (shown: readonly ISlide[], slide: ISlide, index: number): IGhost => ({
  slide,
  index,
  total: shown.length,
  trail: shown.slice(0, index + 1).map((slide) => slide.title),
});

const useGhosts = (slides: readonly ISlide[], reducedMotion: boolean): IGhost[] => {
  const [Ghosts, setGhosts] = useState<IGhost[]>([]);
  const [Shown, setShown] = useState<readonly ISlide[]>(slides);
  const signature = slides.map((slide) => slide.id).join('|');
  const [Signature, setSignature] = useState(signature);

  if (signature !== Signature) {
    const live = new Set(slides.map((slide) => slide.id));
    const removed = Shown.flatMap((slide, index) => (live.has(slide.id) ? [] : [toGhost(Shown, slide, index)]));
    setSignature(signature);
    setShown(slides);
    setGhosts((current) => [...current.filter((ghost) => !live.has(ghost.slide.id)), ...removed]);
  }

  useEffect(() => {
    if (Ghosts.length === 0) return undefined;
    const timer = setTimeout(
      () => {
        setGhosts([]);
      },
      reducedMotion ? 0 : EXIT_MS,
    );
    return () => {
      clearTimeout(timer);
    };
  }, [Ghosts, reducedMotion]);

  return Ghosts;
};

const useFocusFlow = (count: number, topId: string | undefined) => {
  const openers = useRef<(HTMLElement | null)[]>([]);
  const captured = useRef({ count: 0, topId });
  const focused = useRef({ count: 0, topId });
  const topNode = useRef<HTMLElement | null>(null);

  const topRef = useCallback((node: HTMLElement | null) => {
    topNode.current = node;
    node?.addEventListener('keydown', wrapTab);
    return () => {
      node?.removeEventListener('keydown', wrapTab);
      topNode.current = null;
    };
  }, []);

  useLayoutEffect(() => {
    const before = captured.current;
    if (count > before.count) {
      const active = document.activeElement;
      const opener = active instanceof HTMLElement && active !== document.body ? active : null;
      for (let level = before.count; level < count; level += 1) {
        openers.current[level] = level === before.count ? opener : null;
      }
    } else if (count === before.count && count > 0 && topId !== before.topId) {
      openers.current[count - 1] = null;
    }
    captured.current = { count, topId };
  }, [count, topId]);

  useEffect(() => {
    const before = focused.current;
    focused.current = { count, topId };
    if (count > before.count || (count === before.count && count > 0 && topId !== before.topId)) {
      topNode.current?.focus({ preventScroll: true });
    } else if (count < before.count && count > 0) {
      const opener = openers.current[count];
      if (opener?.isConnected) opener.focus({ preventScroll: true });
      else topNode.current?.focus({ preventScroll: true });
      openers.current.length = count;
    }
  }, [count, topId]);

  const takeOriginalOpener = (): HTMLElement | null => {
    const opener = openers.current[0] ?? null;
    openers.current = [];
    captured.current = { count: 0, topId: undefined };
    focused.current = { count: 0, topId: undefined };
    return opener?.isConnected ? opener : null;
  };

  return {
    topRef,
    focusTop: () => {
      topNode.current?.focus({ preventScroll: true });
    },
    takeOriginalOpener,
  };
};

const useGuarded = ({ onPop, onJump, onCloseAll, closeGuard }: Omit<ISlideStackProps, 'slides'>) => {
  const guarded = (close: () => void) => () => {
    if (closeGuard?.blocked === true) closeGuard.onBlocked();
    else close();
  };
  return {
    onPop: guarded(onPop),
    onCloseAll: guarded(onCloseAll),
    onJump: (index: number, source: TJumpSource) => {
      guarded(() => {
        onJump(index, source);
      })();
    },
  };
};

export const SlideStack = ({ slides, ...handlers }: ISlideStackProps) => {
  const { onPop, onCloseAll, onJump } = useGuarded(handlers);
  const reducedMotion = usePrefersReducedMotion();
  const ghosts = useGhosts(slides, reducedMotion);
  const { topRef, focusTop, takeOriginalOpener } = useFocusFlow(slides.length, slides[slides.length - 1]?.id);
  const open = slides.length > 0;
  const backdropOn = useEnterFlag(open, reducedMotion);
  const total = slides.length;

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(next) => {
        if (!next) onPop();
      }}
    >
      <Dialog.Portal forceMount>
        {open || ghosts.length > 0 ? (
          <>
            <div
              role="presentation"
              aria-hidden="true"
              data-on={backdropOn}
              onClick={onPop}
              className={
                'pointer-events-none fixed inset-0 z-20 bg-backdrop opacity-0 ' +
                'transition-opacity duration-[280ms] ease-dhow data-[on=true]:pointer-events-auto data-[on=true]:opacity-100'
              }
            />
            <Dialog.Content
              forceMount
              aria-describedby={undefined}
              style={{ pointerEvents: 'none' }}
              onInteractOutside={(event) => {
                event.preventDefault();
              }}
              onOpenAutoFocus={(event) => {
                event.preventDefault();
                focusTop();
              }}
              onCloseAutoFocus={(event) => {
                event.preventDefault();
                takeOriginalOpener()?.focus({ preventScroll: true });
              }}
              className={
                'fixed inset-y-0 start-menu z-25 w-[min(var(--spacing-panel),calc(100dvw-var(--spacing-menu)-1.5rem))] ' +
                '[--peek:30px] max-md:start-0 max-md:bottom-bottombar max-md:w-full max-md:[--peek:14px]'
              }
            >
              {[
                ...slides.map((slide, index) => (
                  <SlideView
                    key={slide.id}
                    slide={slide}
                    index={index}
                    total={total}
                    trail={slides.slice(0, index + 1).map((entry) => entry.title)}
                    depth={total - 1 - index}
                    leaving={false}
                    reducedMotion={reducedMotion}
                    sectionRef={index === total - 1 ? topRef : undefined}
                    onBack={onPop}
                    onCloseAll={onCloseAll}
                    onCrumb={(position) => {
                      onJump(position, 'crumb');
                    }}
                    onReturn={() => {
                      onJump(index, 'slide');
                    }}
                  />
                )),
                ...ghosts.map((ghost) => (
                  <SlideView
                    key={ghost.slide.id}
                    slide={ghost.slide}
                    index={ghost.index}
                    total={ghost.total}
                    trail={ghost.trail}
                    depth={0}
                    leaving
                    reducedMotion={reducedMotion}
                    onBack={onPop}
                    onCloseAll={onCloseAll}
                    onCrumb={() => undefined}
                    onReturn={() => undefined}
                  />
                )),
              ]}
            </Dialog.Content>
          </>
        ) : null}
      </Dialog.Portal>
    </Dialog.Root>
  );
};

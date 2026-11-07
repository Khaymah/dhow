import * as Dialog from '@radix-ui/react-dialog';
import type { CSSProperties, ReactNode, Ref } from 'react';

import { useEnterFlag } from '@/Core/Hooks/useEnterFlag';
import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import { IconButton } from '@/Core/Ui/IconButton';
import { useT } from '@/I18n/useT';

export interface ISlide {
  id: string;
  title: string;
  subtitle?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}

export interface ISlideViewProps {
  slide: ISlide;
  index: number;
  total: number;
  trail: readonly string[];
  depth: number;
  leaving: boolean;
  reducedMotion: boolean;
  sectionRef?: Ref<HTMLElement>;
  onBack: () => void;
  onCloseAll: () => void;
  onCrumb: (index: number) => void;
  onReturn: () => void;
}

export const SLIDE_BODY = 'grid min-h-0 flex-1 auto-rows-max content-start gap-block overflow-auto px-card py-card';

export const SlideContent = ({ children }: { children: ReactNode }) => <div className="flex flex-col gap-block">{children}</div>;

export const SlidePinned = ({ children }: { children: ReactNode }) => (
  <div data-slide-pinned="" className="sticky -top-card z-10 -mt-card -mb-block bg-canvas pt-card pb-block">
    {children}
  </div>
);

type TTrailProps = Pick<ISlideViewProps, 'trail' | 'index' | 'onCrumb'>;

interface ISlideHeaderProps extends Pick<
  ISlideViewProps,
  'slide' | 'index' | 'total' | 'trail' | 'onBack' | 'onCloseAll' | 'onCrumb'
> {
  top: boolean;
}

const Trail = ({ trail, index, onCrumb }: TTrailProps) => {
  const t = useT();

  return (
    <nav aria-label={t('slides.trail')}>
      <ol className="m-0 flex list-none flex-wrap items-center gap-1.5 p-0 text-body-sm text-tertiary">
        {trail.map((title, position) => (
          <li key={position} className="inline-flex items-center gap-1.5">
            {position > 0 ? <Icon name="next" size={12} className="text-tertiary" /> : null}
            {position === index ? (
              <span aria-current="step" className="text-primary">
                {title}
              </span>
            ) : (
              <button
                type="button"
                onClick={() => {
                  onCrumb(position);
                }}
                className={
                  'min-h-6 min-w-6 cursor-pointer rounded-xs border-0 bg-transparent px-1 py-0.5 text-body-sm text-secondary ' +
                  'hover:bg-hover hover:text-primary'
                }
              >
                {title}
              </button>
            )}
          </li>
        ))}
      </ol>
    </nav>
  );
};

const SlideHeader = ({ slide, index, total, trail, top, onBack, onCloseAll, onCrumb }: ISlideHeaderProps) => {
  const t = useT();
  const heading = (
    <h2 className="m-0 min-w-0 truncate text-title-section">
      <bdi>{slide.title}</bdi>
    </h2>
  );

  return (
    <header className="grid gap-2 border-b border-line px-card pt-4 pb-3.5">
      <div className="flex items-center gap-2.5">
        {index > 0 ? (
          <IconButton icon="back" size="sm" label={t('slides.back')} onClick={onBack} />
        ) : null}
        {top ? <Dialog.Title asChild>{heading}</Dialog.Title> : heading}
        <div className="ms-auto flex items-center gap-2">
          {total > 1 ? (
            <span className="rounded-pill border border-line bg-wash px-2.5 text-caption font-medium whitespace-nowrap text-secondary">
              {t('slides.level', { current: index + 1, total })}
            </span>
          ) : null}
          <IconButton icon="close" size="sm" label={t('slides.closeAll')} onClick={onCloseAll} />
        </div>
      </div>
      {total > 1 ? <Trail trail={trail} index={index} onCrumb={onCrumb} /> : null}
      {slide.subtitle ? (
        <div className="flex flex-wrap items-center gap-x-3.5 gap-y-2 text-body-sm text-secondary">
          {slide.subtitle}
        </div>
      ) : null}
    </header>
  );
};

export const SlideView = ({
  slide,
  index,
  total,
  trail,
  depth,
  leaving,
  reducedMotion,
  sectionRef,
  onBack,
  onCloseAll,
  onCrumb,
  onReturn,
}: ISlideViewProps) => {
  const entered = useEnterFlag(true, reducedMotion);
  const top = depth === 0 && !leaving;
  const under = !top && !leaving;

  return (
    <div
      data-slide-level={index}
      data-in={entered && !leaving}
      data-under={under}
      data-gone={depth > 3 || undefined}
      data-leaving={leaving || undefined}
      style={
        {
          '--depth': depth,
          pointerEvents: leaving ? 'none' : 'auto',
        } as CSSProperties
      }
      className={cx(
        'absolute inset-y-3 start-3 end-0 max-md:start-1.5 max-md:end-1.5',
        'slide-offscreen transition-[transform,filter] duration-[380ms] ease-dhow will-change-transform',
        'ltr:origin-right rtl:origin-left',
        'data-[in=true]:slide-shift',
        'data-[under=true]:brightness-[calc(1_-_var(--depth)_*_0.22)]',
        'data-[gone=true]:invisible',
      )}
    >
      <section
        ref={sectionRef}
        tabIndex={-1}
        inert={!top}
        aria-hidden={top ? undefined : true}
        className={cx(
          'flex size-full flex-col overflow-hidden rounded-lg border border-line-2 bg-canvas shadow-overlay',
        )}
      >
        <SlideHeader
          slide={slide}
          index={index}
          total={total}
          trail={trail}
          top={top}
          onBack={onBack}
          onCloseAll={onCloseAll}
          onCrumb={onCrumb}
        />
        <div className={SLIDE_BODY}>{slide.children}</div>
        {slide.footer ? (
          <footer className="flex flex-wrap items-center gap-2 border-t border-line px-card py-3">{slide.footer}</footer>
        ) : null}
      </section>
      {under ? (
        <button
          type="button"
          tabIndex={-1}
          aria-hidden="true"
          data-slide-return=""
          onClick={onReturn}
          className="absolute inset-0 cursor-pointer rounded-lg border-0 bg-transparent p-0"
        />
      ) : null}
    </div>
  );
};

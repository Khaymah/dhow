import { useId } from 'react';

import { ACCENT_DOT, type TAccent } from '@/Core/Ui/Accent';
import { cx } from '@/Core/Ui/ClassNames';
import { ErrorHelp } from '@/Core/Ui/ErrorHelp';

export interface IToastProps {
  accent?: TAccent;
  title: string;
  detail?: string;
  fix?: string;
  helpHref?: string | null;
  dismissLabel: string;
  onDismiss: () => void;
}

export const Toast = ({ accent = 'live', title, detail, fix = '', helpHref = null, dismissLabel, onDismiss }: IToastProps) => {
  const fixId = useId();
  return (
  <div
    role="status"
    aria-describedby={fix === '' ? undefined : fixId}
    data-accent={accent}
    className="flex min-w-[300px] items-center gap-3 rounded-lg border border-line-2 bg-surface px-4 py-3 text-body-sm shadow-overlay"
  >
    <i aria-hidden="true" className={cx('size-2 flex-none rounded-full', ACCENT_DOT[accent])} />
    <span className="grid flex-1 gap-px">
      {title}
      {detail ? <span className="text-body-sm text-secondary">{detail}</span> : null}
      {fix !== '' || helpHref !== null ? <ErrorHelp fix={fix} helpHref={helpHref} fixId={fixId} /> : null}
    </span>
    <button
      type="button"
      onClick={onDismiss}
      className="h-btn-sm cursor-pointer rounded-btn border-0 bg-transparent px-2.5 text-label-sm text-secondary hover:bg-hover hover:text-primary"
    >
      {dismissLabel}
    </button>
  </div>
  );
};

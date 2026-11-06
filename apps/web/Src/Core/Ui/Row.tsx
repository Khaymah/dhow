import type { ReactNode } from 'react';

import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import type { TIconName } from '@/Core/Ui/Icons';

export interface IRowProps {
  title: ReactNode;
  sub?: ReactNode;
  icon?: TIconName;
  start?: ReactNode;
  end?: ReactNode;
  mono?: boolean;
  onSelect?: () => void;
}

export interface IRowsProps {
  label?: string;
  children: ReactNode;
}

const INNER = 'flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 px-card py-3';

export const Rows = ({ label, children }: IRowsProps) => (
  <ul aria-label={label} className="m-0 grid list-none p-0">
    {children}
  </ul>
);

export const Row = ({ title, sub, icon, start, end, mono = false, onSelect }: IRowProps) => {
  const body = (
    <>
      {icon ? <Icon name={icon} size={18} className="text-secondary" /> : null}
      {start}
      <span className="grid min-w-0 flex-1 basis-48 gap-0.5">
        <span className={cx('min-w-0 text-label', mono && 'font-mono')}>{mono ? <bdi dir="ltr">{title}</bdi> : title}</span>
        {sub ? <small className="min-w-0 text-body-sm text-tertiary">{sub}</small> : null}
      </span>
      {end ? <span className="ms-auto flex max-w-full flex-wrap items-center gap-1.5">{end}</span> : null}
      {onSelect ? <Icon name="next" size={16} className="flex-none text-tertiary" /> : null}
    </>
  );

  return (
    <li className="min-w-0 border-t border-line first:border-t-0">
      {onSelect ? (
        <button
          type="button"
          onClick={onSelect}
          className={cx(INNER, 'w-full cursor-pointer border-0 bg-transparent text-start text-primary hover:bg-surface-raised [li:last-child_&]:rounded-b-md')}
        >
          {body}
        </button>
      ) : (
        <div className={INNER}>{body}</div>
      )}
    </li>
  );
};

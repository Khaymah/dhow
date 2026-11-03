import type { ReactNode } from 'react';

import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import type { TIconName } from '@/Core/Ui/Icons';
import { followInApp } from '@/Core/Ui/PlainClick';

export interface ILinkRowProps {
  icon: TIconName;
  title: ReactNode;
  sub?: ReactNode;
  end?: ReactNode;
  href: string;
  selected?: boolean;
  onOpen: () => void;
}

export const LinkRow = ({ icon, title, sub, end, href, selected = false, onOpen }: ILinkRowProps) => (
  <li className="min-w-0 border-t border-line first:border-t-0">
    <a
      href={href}
      aria-current={selected ? 'page' : undefined}
      onClick={followInApp(onOpen)}
      className={cx(
        'flex min-w-0 items-center gap-3 border-s-2 px-card py-3 text-primary no-underline transition-colors duration-140 [li:last-child_&]:rounded-b-md',
        selected ? 'border-accent bg-sel' : 'border-transparent hover:bg-surface-raised',
      )}
    >
      <Icon name={icon} size={18} className={selected ? 'text-accent' : 'text-secondary'} />
      <span className="grid min-w-0 flex-1 gap-0.5">
        <span className="min-w-0 truncate text-label">{title}</span>
        {sub === undefined ? null : <small className="min-w-0 text-body-sm text-tertiary">{sub}</small>}
      </span>
      {end === undefined ? null : <span className="flex flex-none flex-wrap items-center justify-end gap-1.5">{end}</span>}
      <Icon name="next" size={16} className="flex-none text-tertiary" />
    </a>
  </li>
);

import type { ReactNode } from 'react';

import { Icon } from '@/Core/Ui/Icon';
import type { TIconName } from '@/Core/Ui/Icons';
import { followInApp } from '@/Core/Ui/PlainClick';

export interface ILinkRowProps {
  icon: TIconName;
  title: ReactNode;
  sub?: ReactNode;
  end?: ReactNode;
  href: string;
  onOpen: () => void;
}

export const LinkRow = ({ icon, title, sub, end, href, onOpen }: ILinkRowProps) => (
  <li className="min-w-0 border-t border-line first:border-t-0">
    <a
      href={href}
      onClick={followInApp(onOpen)}
      className="flex min-w-0 items-center gap-3 px-card py-3 text-primary no-underline transition-colors duration-140 hover:bg-surface-raised [li:last-child_&]:rounded-b-md"
    >
      <Icon name={icon} size={18} className="flex-none text-secondary" />
      <span className="grid min-w-0 flex-1 gap-0.5">
        <span className="min-w-0 text-label">{title}</span>
        {sub === undefined ? null : <small className="min-w-0 text-body-sm text-tertiary">{sub}</small>}
      </span>
      {end === undefined ? null : <span className="flex flex-none flex-wrap items-center justify-end gap-1.5">{end}</span>}
      <Icon name="next" size={16} className="flex-none text-tertiary" />
    </a>
  </li>
);

import type { ComponentProps } from 'react';

import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import type { TIconName } from '@/Core/Ui/Icons';

export interface IIconButtonProps extends Omit<ComponentProps<'button'>, 'children'> {
  icon: TIconName;
  label: string;
  size?: 'sm' | 'md';
  plain?: boolean;
  badge?: boolean;
  count?: number;
}

const MAX_COUNT = 99;
const SIZES = { sm: 'size-9', md: 'size-btn' } as const;
const GLYPH_SIZES = { sm: 16, md: 18 } as const;

export const IconButton = ({
  icon,
  label,
  size = 'md',
  plain = false,
  badge = false,
  count = 0,
  className,
  type = 'button',
  ...rest
}: IIconButtonProps) => (
  <button
    type={type}
    aria-label={label}
    className={cx(
      'relative inline-grid flex-none cursor-pointer place-items-center rounded-btn border text-secondary',
      'transition-colors duration-140 hover:text-primary',
      plain ? 'border-transparent bg-transparent hover:bg-hover' : 'border-line-2 bg-transparent hover:bg-surface-raised',
      SIZES[size],
      className,
    )}
    {...rest}
  >
    <Icon name={icon} size={GLYPH_SIZES[size]} />
    {badge ? <i aria-hidden="true" className="absolute end-2 top-2 size-2 rounded-full bg-accent-danger" /> : null}
    {count > 0 ? (
      <span
        aria-hidden="true"
        className="absolute -end-1.5 -top-1.5 grid h-5 min-w-5 place-items-center rounded-pill bg-accent px-1 text-caption font-medium text-on-accent tabular-nums"
      >
        {count > MAX_COUNT ? `${MAX_COUNT}+` : count}
      </span>
    ) : null}
  </button>
);

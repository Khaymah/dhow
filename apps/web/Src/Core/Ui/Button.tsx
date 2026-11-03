import type { ComponentProps } from 'react';

import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import type { TIconName } from '@/Core/Ui/Icons';

export type TButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

interface IButtonBase extends ComponentProps<'button'> {
  size?: 'sm' | 'md';
}

export type TButtonProps =
  | (IButtonBase & { variant?: Exclude<TButtonVariant, 'danger'>; iconStart?: TIconName })
  | (IButtonBase & { variant: 'danger'; iconStart: TIconName });

const VARIANTS: Record<TButtonVariant, string> = {
  primary: 'border-transparent bg-accent text-on-accent hover:brightness-110 disabled:bg-surface-raised disabled:text-muted disabled:brightness-100',
  secondary:
    'border-line-2 bg-transparent text-primary hover:bg-surface-raised disabled:border-line disabled:bg-transparent disabled:text-muted',
  ghost: 'border-transparent bg-transparent text-secondary hover:bg-hover hover:text-primary disabled:bg-transparent disabled:text-muted',
  danger: 'border-transparent bg-transparent text-accent-coral hover:bg-bad-wash disabled:bg-transparent disabled:text-muted',
};

export const Button = ({
  variant = 'secondary',
  size = 'md',
  iconStart,
  className,
  children,
  type = 'button',
  ...rest
}: TButtonProps) => (
  <button
    type={type}
    data-variant={variant}
    className={cx(
      'inline-flex cursor-pointer items-center justify-center gap-2 rounded-btn border whitespace-nowrap',
      'transition-colors duration-140 ease-dhow disabled:cursor-not-allowed max-md:min-h-11',
      size === 'sm' ? 'h-btn-sm px-3 text-label-sm' : 'h-btn px-4 text-label',
      VARIANTS[variant],
      className,
    )}
    {...rest}
  >
    {iconStart ? <Icon name={iconStart} size={size === 'sm' ? 15 : 16} /> : null}
    {children}
  </button>
);

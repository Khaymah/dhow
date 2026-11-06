import type { ReactNode } from 'react';

import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';

export interface IChipProps {
  tone?: 'mute' | 'warn';
  protectedMark?: boolean;
  children: ReactNode;
}

const TONES = {
  mute: 'border-line bg-wash text-secondary',
  warn: 'border-warn-line bg-warn-wash text-warn-fg',
} as const;

export const Chip = ({ tone = 'mute', protectedMark = false, children }: IChipProps) => (
  <span
    className={cx(
      'inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-pill border px-2.5 py-0.5 text-caption font-medium break-all',
      TONES[tone],
    )}
  >
    {protectedMark ? <Icon name="shield" size={13} weight={2.4} className="flex-none" /> : null}
    <span className="min-w-0">{children}</span>
  </span>
);

import { useEffect, useRef, type ReactNode } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { formatClock } from '@/Core/Format/TimeFormat';
import type { IFactorAction } from '@/Core/Hooks/useFactorAction';
import { Button } from '@/Core/Ui/Button';
import { FactorFields } from '@/Core/Ui/FactorFields';
import type { TIconName } from '@/Core/Ui/Icons';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';

export interface IConfirmPanelProps<TResult> {
  question: string;
  confirm: string;
  icon: TIconName;
  danger?: boolean;
  disabled?: boolean;
  children?: ReactNode;
  methods: readonly TFactorMethod[];
  action: IFactorAction<TResult>;
  onConfirm: () => void;
  onCancel: () => void;
}

export const ConfirmPanel = <TResult,>({
  question,
  confirm,
  icon,
  danger = false,
  disabled = false,
  children,
  methods,
  action,
  onConfirm,
  onCancel,
}: IConfirmPanelProps<TResult>) => {
  const t = useT();
  const panelRef = useRef<HTMLDivElement>(null);
  const locked = action.Countdown.Remaining > 0;

  useEffect(() => {
    panelRef.current?.querySelector<HTMLElement>('input, button')?.focus();
  }, []);

  return (
    <div ref={panelRef} className="grid gap-3 px-card pb-card">
      <p className="m-0 text-body-sm text-secondary">{question}</p>
      {children}
      {action.Notice ? <InlineAlert focus accent="failed" title={action.Notice} /> : null}
      <FactorFields methods={methods} draft={action.Draft} onChange={action.SetDraft} disabled={action.Busy} />
      <div className="flex flex-wrap gap-2">
        {danger ? (
          <Button size="sm" variant="danger" iconStart={icon} disabled={action.Busy || locked || disabled} onClick={onConfirm}>
            {locked ? t('throttle.retryIn', { time: formatClock(action.Countdown.Remaining) }) : confirm}
          </Button>
        ) : (
          <Button size="sm" variant="primary" iconStart={icon} disabled={action.Busy || locked || disabled} onClick={onConfirm}>
            {locked ? t('throttle.retryIn', { time: formatClock(action.Countdown.Remaining) }) : confirm}
          </Button>
        )}
        <Button size="sm" variant="ghost" disabled={action.Busy} onClick={onCancel}>
          {t('mfa.cancel')}
        </Button>
      </div>
    </div>
  );
};

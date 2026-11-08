import type { TFactorMethod } from '@/Core/Auth/Factor';
import { formatClock } from '@/Core/Format/TimeFormat';
import type { IFactorAction } from '@/Core/Hooks/useFactorAction';
import { Button } from '@/Core/Ui/Button';
import { FactorFields } from '@/Core/Ui/FactorFields';
import { useT } from '@/I18n/useT';

export interface IFactorSubmitProps {
  methods: readonly TFactorMethod[];
  action: IFactorAction<unknown>;
  label: string;
  busyLabel: string;
}

export const FactorSubmit = ({ methods, action, label, busyLabel }: IFactorSubmitProps) => {
  const t = useT();
  const locked = action.Countdown.Remaining > 0;
  const text = locked ? t('throttle.retryIn', { time: formatClock(action.Countdown.Remaining) }) : action.Busy ? busyLabel : label;

  return (
    <div className="grid gap-3 border-t border-line pt-4">
      <p className="m-0 text-body-sm text-secondary">{t('notifications.factor.about')}</p>
      <FactorFields methods={methods} draft={action.Draft} onChange={action.SetDraft} disabled={action.Busy} />
      <Button type="submit" variant="primary" iconStart="key" className="justify-self-start" disabled={action.Busy || locked}>
        {text}
      </Button>
    </div>
  );
};

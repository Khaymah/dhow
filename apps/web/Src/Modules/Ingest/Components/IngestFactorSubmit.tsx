import type { TFactorMethod } from '@/Core/Auth/Factor';
import { formatClock } from '@/Core/Format/TimeFormat';
import type { IFactorAction } from '@/Core/Hooks/useFactorAction';
import { Button } from '@/Core/Ui/Button';
import { FactorFields } from '@/Core/Ui/FactorFields';
import { useT } from '@/I18n/useT';

export interface IIngestFactorSubmitProps<TResult> {
  methods: readonly TFactorMethod[];
  action: IFactorAction<TResult>;
  label: string;
  busyLabel: string;
  disabled?: boolean;
}

export const IngestFactorSubmit = <TResult,>({ methods, action, label, busyLabel, disabled = false }: IIngestFactorSubmitProps<TResult>) => {
  const t = useT();
  const locked = action.Countdown.Remaining > 0;
  const text = locked ? t('throttle.retryIn', { time: formatClock(action.Countdown.Remaining) }) : action.Busy ? busyLabel : label;

  return (
    <div className="grid gap-3 border-t border-line pt-4">
      <p className="m-0 text-body-sm text-secondary">{t('ingest.factor.about')}</p>
      <FactorFields methods={methods} draft={action.Draft} onChange={action.SetDraft} disabled={action.Busy} />
      <Button type="submit" variant="primary" iconStart="key" className="justify-self-start" disabled={action.Busy || locked || disabled}>
        {text}
      </Button>
    </div>
  );
};

import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { StatusDot } from '@/Core/Ui/StatusDot';
import { useT } from '@/I18n/useT';
import type { TRun } from '@/Modules/Projects/Api/RunsApi';
import { LinkRow } from '@/Modules/Projects/Components/LinkRow';
import { useRunText } from '@/Modules/Projects/Hooks/useRunText';
import { isFinalRun, runStatusOf, shortRef, shortSha } from '@/Modules/Projects/Types/RunStates';

export interface IRunRowProps {
  run: TRun;
  now: number;
  href: string;
  onOpen: () => void;
}

export const RunRow = ({ run, now, href, onOpen }: IRunRowProps) => {
  const t = useT();
  const time = useRelativeTime();
  const text = useRunText();
  const took = text.runDuration(run, now);
  const parts = [
    t(`runs.kind.${run.Trigger}`),
    t('runs.row.by', { actor: text.actor(run) }),
    time.ago(run.CreatedAt),
    took === '' ? '' : isFinalRun(run.State) ? t('runs.row.took', { duration: took }) : t('runs.row.runningFor', { duration: took }),
  ].filter((part) => part !== '');

  return (
    <LinkRow
      icon="play"
      href={href}
      onOpen={onOpen}
      title={
        <span className="inline-flex min-w-0 flex-wrap items-center gap-x-2">
          <span>{t('runs.row.number', { number: run.Number })}</span>
          <bdi dir="ltr" className="font-mono text-secondary">
            {shortRef(run.Ref)}
          </bdi>
          {run.Sha === undefined ? null : (
            <bdi dir="ltr" className="font-mono text-tertiary">
              {shortSha(run.Sha)}
            </bdi>
          )}
        </span>
      }
      sub={
        <>
          <span>{parts.join(' · ')}</span>
          {run.FailureReason === undefined || run.FailureReason === '' || run.State === 'succeeded' ? null : (
            <span className="block text-secondary">{text.reason(run.FailureReason)}</span>
          )}
        </>
      }
      end={
        <>
          {run.IsFork ? (
            <Badge tone="warn" icon="alert">
              {t('runs.fork')}
            </Badge>
          ) : null}
          <StatusDot status={runStatusOf(run.State)}>{t(`runs.state.${run.State}`)}</StatusDot>
        </>
      }
    />
  );
};

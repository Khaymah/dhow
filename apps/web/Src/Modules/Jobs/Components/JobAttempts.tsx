import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { StatusDot } from '@/Core/Ui/StatusDot';
import { useToastStore } from '@/Core/Store/useToastStore';
import { copyText } from '@/Core/Ui/Clipboard';
import { useT } from '@/I18n/useT';
import type { TJobAttempt, TJobAttemptStep, TJobStepState } from '@/Modules/Jobs/Api/JobsApi';
import { AttemptStateDot } from '@/Modules/Jobs/Components/JobStateDot';
import { useJobText } from '@/Modules/Jobs/Hooks/useJobText';
import { useNow } from '@/Modules/Jobs/Hooks/useNow';
import { isFinalState } from '@/Modules/Jobs/Types/JobStates';

const STEP_STATUS = {
  running: 'building',
  succeeded: 'live',
  failed: 'failed',
  interrupted: 'rolledback',
} as const satisfies Record<TJobStepState, string>;

const StepLine = ({ step, now }: { step: TJobAttemptStep; now: number }) => {
  const t = useT();
  const text = useJobText();
  const extra = [
    text.duration(step.StartedAt, step.FinishedAt ?? now),
    step.ExitCode === undefined ? '' : t('jobs.attempts.exitCode', { code: step.ExitCode }),
    step.FailureReason === undefined ? '' : text.reason(step.FailureReason),
  ].filter((part) => part !== '');
  return (
    <li className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 py-1.5 text-body-sm">
      <span className="min-w-0">
        <span className="text-secondary">{t('jobs.spec.step', { n: step.Index + 1 })}</span> <bdi>{step.Name}</bdi>
        <small className="block text-caption text-tertiary">{extra.join(' · ')}</small>
      </span>
      <StatusDot status={STEP_STATUS[step.State]}>{t(`jobs.step.state.${step.State}`)}</StatusDot>
    </li>
  );
};

const AttemptItem = ({ attempt, now }: { attempt: TJobAttempt; now: number }) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const time = useRelativeTime();
  const text = useJobText();
  const facts = [
    t('jobs.attempts.leasedAt', { time: time.ago(attempt.LeasedAt) }),
    attempt.StartedAt === undefined ? '' : text.duration(attempt.StartedAt, attempt.FinishedAt ?? now),
    attempt.ExitCode === undefined ? '' : t('jobs.attempts.exitCode', { code: attempt.ExitCode }),
  ].filter((part) => part !== '');

  return (
    <li className="grid gap-2 border-t border-line px-4 py-3 first:border-t-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-label">{t('jobs.attempts.number', { n: attempt.Number })}</span>
        <AttemptStateDot state={attempt.State} />
      </div>
      <small className="text-caption text-tertiary">{facts.join(' · ')}</small>
      <small className="flex flex-wrap items-center gap-x-2 text-caption text-tertiary">
        {t('jobs.attempts.runner')}
        <bdi className="text-secondary">{attempt.RunnerName ?? t('jobs.attempts.runnerGone')}</bdi>
        <button
          type="button"
          className="inline-flex min-h-11 cursor-pointer items-center gap-1 rounded-xs border-0 bg-transparent p-0 text-caption text-tertiary underline decoration-line-2 underline-offset-4 hover:text-primary md:min-h-0"
          onClick={() => {
            void copyText(attempt.RunnerId).then((done) => {
              Push(
                done
                  ? { Id: `runner-id-${attempt.Id}`, Accent: 'live', Title: t('jobs.attempts.runnerId.copied') }
                  : { Id: `runner-id-failed-${attempt.Id}`, Accent: 'awaiting', Title: t('jobs.attempts.runnerId.failed') },
              );
            });
          }}
        >
          {t('jobs.attempts.runnerId.copy')}
        </button>
      </small>
      {attempt.FailureReason === undefined ? null : <p className="m-0 text-body-sm">{text.reason(attempt.FailureReason)}</p>}
      {attempt.MutationStartedAt === undefined ? null : (
        <p className="m-0 text-body-sm text-warn-fg">{t('jobs.attempts.mutation')}</p>
      )}
      {attempt.Steps.length > 0 ? (
        <ol className="m-0 grid list-none divide-y divide-line p-0">
          {attempt.Steps.map((step) => (
            <StepLine key={step.Index} step={step} now={now} />
          ))}
        </ol>
      ) : null}
    </li>
  );
};

export const JobAttempts = ({ attempts, maxRetries }: { attempts: readonly TJobAttempt[]; maxRetries: number }) => {
  const t = useT();
  const now = useNow(attempts.some((attempt) => !isFinalState(attempt.State)));
  const ordered = [...attempts].sort((a, b) => b.Number - a.Number);
  return (
    <Card title={t('jobs.attempts.title')} icon="layers">
      {ordered.length === 0 ? (
        <EmptyState icon="queue" text={t('jobs.attempts.none')} />
      ) : (
        <ol aria-label={t('jobs.attempts.title')} className="m-0 grid list-none p-0">
          {ordered.map((attempt) => (
            <AttemptItem key={attempt.Id} attempt={attempt} now={now} />
          ))}
        </ol>
      )}
      <p className="m-0 border-t border-line px-4 py-2.5 text-body-sm text-tertiary">
        {t('jobs.attempts.retries', { count: maxRetries })}
      </p>
    </Card>
  );
};

import type { ReactNode } from 'react';

import type { TRole } from '@/Core/Auth/AuthTypes';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Card } from '@/Core/Ui/Card';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TJob } from '@/Modules/Jobs/Api/JobsApi';
import { CancelJob } from '@/Modules/Jobs/Components/CancelJob';
import { Facts, type IFact } from '@/Modules/Jobs/Components/Facts';
import { GVISOR } from '@/Modules/Jobs/Components/JobSpecCard';
import { JobStateDot } from '@/Modules/Jobs/Components/JobStateDot';
import { useJobText } from '@/Modules/Jobs/Hooks/useJobText';
import { useNow } from '@/Modules/Jobs/Hooks/useNow';
import { canCancelJobs, isCancelable, isFinalState, isStuck } from '@/Modules/Jobs/Types/JobStates';

export interface IJobOverviewProps {
  organizationId: string;
  job: TJob;
  role: TRole | undefined;
  name?: string;
  origin?: ReactNode;
  waitingFor?: (job: TJob) => ReactNode;
}

export const NoRunnerAlert = ({ job }: { job: TJob }) => {
  const t = useT();
  const time = useRelativeTime();
  return (
    <InlineAlert accent="awaiting" title={t('jobs.noRunner.title')}>
      <span className="grid gap-1.5">
        <span>{t('jobs.noRunner.about', { isolation: t(`jobs.spec.isolation.${job.Isolation}`, { name: GVISOR }) })}</span>
        {job.Labels.length > 0 ? (
          <span className="flex flex-wrap items-center gap-1.5">
            {t('jobs.noRunner.labels')}
            {job.Labels.map((label) => (
              <Badge key={label} tone="mute">
                <bdi dir="ltr" className="font-mono">
                  {label}
                </bdi>
              </Badge>
            ))}
          </span>
        ) : null}
        <span>{t('jobs.noRunner.how')}</span>
        <span>{t('jobs.noRunner.deadline', { time: time.full(job.QueueDeadline) })}</span>
      </span>
    </InlineAlert>
  );
};

const useFacts = (job: TJob): IFact[] => {
  const t = useT();
  const time = useRelativeTime();
  const text = useJobText();
  const final = isFinalState(job.State);
  const now = useNow(!final);
  const at = (iso: string) => <time dateTime={iso}>{time.full(iso)}</time>;
  const facts: IFact[] = [{ id: 'queued', label: t('jobs.detail.queuedAt'), value: at(job.QueuedAt) }];
  if (job.StartedAt !== undefined) facts.push({ id: 'started', label: t('jobs.detail.startedAt'), value: at(job.StartedAt) });
  if (job.FinishedAt !== undefined) facts.push({ id: 'finished', label: t('jobs.detail.finishedAt'), value: at(job.FinishedAt) });
  if (job.StartedAt !== undefined) {
    facts.push({ id: 'duration', label: t('jobs.detail.duration'), value: text.duration(job.StartedAt, job.FinishedAt ?? now) });
  }
  if (!final && job.Deadline !== undefined) facts.push({ id: 'deadline', label: t('jobs.detail.deadline'), value: at(job.Deadline) });
  if (job.State === 'queued') facts.push({ id: 'queue-deadline', label: t('jobs.detail.queueDeadline'), value: at(job.QueueDeadline) });
  if (job.Priority !== 0) facts.push({ id: 'priority', label: t('jobs.detail.priority'), value: job.Priority });
  return facts;
};

export const JobOverview = ({ organizationId, job, role, waitingFor, name, origin }: IJobOverviewProps) => {
  const t = useT();
  const text = useJobText();
  const facts = useFacts(job);
  const final = isFinalState(job.State);

  return (
    <>
      {isStuck(job) ? <NoRunnerAlert job={job} /> : null}
      {isStuck(job) ? waitingFor?.(job) : null}
      {job.State === 'queued' && !isStuck(job) ? (
        <InlineAlert accent="queued" title={t('jobs.waiting.title')}>
          {t('jobs.waiting.about')}
        </InlineAlert>
      ) : null}
      {job.CancelRequestedAt !== undefined && !final ? <InlineAlert accent="building" title={t('jobs.detail.cancelPending')} /> : null}
      <Card padded>
        <div className="grid gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="m-0 min-w-0 text-title-card break-words">
              <bdi>{name ?? job.Name}</bdi>
            </h3>
            <JobStateDot job={job} />
          </div>
          {origin}
          {job.FailureReason === undefined || isStuck(job) ? null : (
            <p className="m-0 text-body-sm">
              <span className="text-secondary">{t('jobs.detail.reason')}: </span>
              {text.reason(job.FailureReason)}
            </p>
          )}
          <Facts facts={facts} />
          {canCancelJobs(role) && isCancelable(job) ? <CancelJob organizationId={organizationId} job={job} /> : null}
        </div>
      </Card>
    </>
  );
};

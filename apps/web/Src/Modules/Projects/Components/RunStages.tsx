import { Card } from '@/Core/Ui/Card';
import { StatusDot } from '@/Core/Ui/StatusDot';
import { useT } from '@/I18n/useT';
import type { TRunJob } from '@/Modules/Projects/Api/RunsApi';
import { useRunText } from '@/Modules/Projects/Hooks/useRunText';
import { jobStatusOf } from '@/Modules/Projects/Types/RunStates';
import { matrixOf, stagesOf } from '@/Modules/Projects/Types/Stages';

export interface IRunStagesProps {
  jobs: readonly TRunJob[];
  now: number;
  jobHref: (jobId: string) => string;
  onOpenJob: (jobId: string) => void;
}

const JobEntry = ({ job, now, jobHref, onOpenJob }: { job: TRunJob } & Omit<IRunStagesProps, 'jobs'>) => {
  const t = useT();
  const text = useRunText();
  const matrix = matrixOf(job);
  const took = job.StartedAt === undefined ? '' : text.duration(job.StartedAt, job.FinishedAt ?? now);
  const reason = job.FailureReason === undefined || job.FailureReason === '' ? null : text.reason(job.FailureReason);
  const title = <bdi>{job.Name === '' ? job.JobId : job.Name}</bdi>;
  const target = job.CurrentJobId;

  return (
    <li className="grid min-w-0 gap-1.5 border-t border-line px-card py-3 first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
        <span className="min-w-0 text-label">
          {target === undefined ? (
            title
          ) : (
            <a
              href={jobHref(target)}
              className="text-primary underline decoration-line-2 underline-offset-4 hover:decoration-accent"
              onClick={(event) => {
                if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return;
                event.preventDefault();
                onOpenJob(target);
              }}
            >
              {title}
            </a>
          )}
        </span>
        <StatusDot status={jobStatusOf(job.State)}>{t(`runs.job.state.${job.State}`)}</StatusDot>
      </div>
      <dl className="m-0 flex flex-wrap gap-x-4 gap-y-1 text-body-sm text-tertiary">
        {matrix.length === 0 ? null : (
          <div className="flex gap-1.5">
            <dt>{t('runs.job.matrix')}</dt>
            <dd className="m-0">
              <bdi dir="ltr" className="font-mono">
                {matrix.join(', ')}
              </bdi>
            </dd>
          </div>
        )}
        <div className="flex gap-1.5">
          <dt>{t('runs.job.needs')}</dt>
          <dd className="m-0">
            {job.Needs.length === 0 ? (
              t('runs.job.needs.none')
            ) : (
              <bdi dir="ltr" className="font-mono">
                {job.Needs.join(', ')}
              </bdi>
            )}
          </dd>
        </div>
        {took === '' ? null : (
          <div className="flex gap-1.5">
            <dt>{t('runs.job.took')}</dt>
            <dd className="m-0">{took}</dd>
          </div>
        )}
        {job.Attempts > 1 ? (
          <div className="flex gap-1.5">
            <dt>{t('runs.job.attempts')}</dt>
            <dd className="m-0">{job.Attempts}</dd>
          </div>
        ) : null}
      </dl>
      {reason === null ? null : <p className="m-0 text-body-sm text-secondary">{reason}</p>}
      {target === undefined && job.State === 'waiting' ? <p className="m-0 text-body-sm text-tertiary">{t('runs.job.notStarted')}</p> : null}
    </li>
  );
};

export const RunStages = ({ jobs, now, jobHref, onOpenJob }: IRunStagesProps) => {
  const t = useT();
  if (jobs.length === 0) {
    return (
      <Card title={t('runs.stages.title')} icon="layers" padded>
        <p className="m-0 text-body-sm text-secondary">{t('runs.stages.empty')}</p>
      </Card>
    );
  }
  return (
    <ol aria-label={t('runs.stages.title')} className="m-0 grid list-none gap-block p-0">
      {stagesOf(jobs).map((stage, index) => (
        <li key={stage.Index}>
          <Card title={t('runs.stage.title', { number: index + 1 })} icon="layers" end={t('runs.stage.count', { count: stage.Jobs.length })}>
            <ul aria-label={t('runs.stage.title', { number: index + 1 })} className="m-0 grid list-none p-0">
              {stage.Jobs.map((job) => (
                <JobEntry key={job.Id} job={job} now={now} jobHref={jobHref} onOpenJob={onOpenJob} />
              ))}
            </ul>
          </Card>
        </li>
      ))}
    </ol>
  );
};

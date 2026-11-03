import type { ReactNode } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import type { TRole } from '@/Core/Auth/AuthTypes';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useCrumbTitle } from '@/Core/Hooks/useCrumbTitle';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { StatusDot } from '@/Core/Ui/StatusDot';
import { useT } from '@/I18n/useT';
import type { TRunDetail } from '@/Modules/Projects/Api/RunsApi';
import { ArtifactsCard } from '@/Modules/Projects/Components/ArtifactsCard';
import { RunActions } from '@/Modules/Projects/Components/RunActions';
import { RunStages } from '@/Modules/Projects/Components/RunStages';
import { useNow } from '@/Modules/Projects/Hooks/useNow';
import { useRunText } from '@/Modules/Projects/Hooks/useRunText';
import { useRun } from '@/Modules/Projects/Hooks/useRuns';
import { canOnProject } from '@/Modules/Projects/Types/ProjectAccess';
import { isKnownIssue } from '@/Modules/Projects/Types/IssueCodes';
import { isFinalRun, runStatusOf, shortRef, shortSha } from '@/Modules/Projects/Types/RunStates';

export interface IRunBodyProps {
  organizationId: string;
  projectId: string;
  runId: string;
  role: TRole;
  linked: boolean;
  crumbKey: string;
  jobHref: (jobId: string) => string;
  onOpenJob: (jobId: string) => void;
  onSecondFactor: () => void;
}

const Fact = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="grid min-w-0 gap-0.5">
    <dt className="text-body-sm text-tertiary">{label}</dt>
    <dd className="m-0 min-w-0 text-body-sm text-primary">{children}</dd>
  </div>
);

const RunFacts = ({ detail, now }: { detail: TRunDetail; now: number }) => {
  const t = useT();
  const time = useRelativeTime();
  const text = useRunText();
  const { Run: run } = detail;
  const took = text.runDuration(run, now);
  return (
    <dl className="m-0 grid grid-cols-2 gap-4 sm:grid-cols-3">
      <Fact label={t('runs.facts.ref')}>
        <bdi dir="ltr" className="break-all font-mono">
          {shortRef(run.Ref)}
        </bdi>
      </Fact>
      <Fact label={t('runs.facts.sha')}>
        <bdi dir="ltr" className="font-mono" title={run.Sha}>
          {run.Sha === undefined ? t('runs.facts.none') : shortSha(run.Sha)}
        </bdi>
      </Fact>
      <Fact label={t('runs.facts.trigger')}>{t(`runs.kind.${run.Trigger}`)}</Fact>
      <Fact label={t('runs.facts.actor')}>
        <bdi>{text.actor(run)}</bdi>
      </Fact>
      <Fact label={t('runs.facts.created')}>{time.full(run.CreatedAt)}</Fact>
      <Fact label={t('runs.facts.duration')}>
        {took === '' ? t('runs.facts.notStarted') : isFinalRun(run.State) ? took : t('runs.facts.runningFor', { duration: took })}
      </Fact>
    </dl>
  );
};

const Issues = ({ detail }: { detail: TRunDetail }) => {
  const t = useT();
  if (detail.Issues.length === 0) return null;
  return (
    <Card title={t('runs.issues.title')} icon="alert">
      <ul aria-label={t('runs.issues.title')} className="m-0 grid list-none p-0">
        {detail.Issues.map((issue) => (
          <li key={`${issue.Code}-${issue.Path}-${issue.Line}-${issue.Column}`} className="grid gap-1 border-t border-line px-card py-3 first:border-t-0">
            <span className="flex flex-wrap items-center gap-2 text-label">
              <Badge tone={issue.Severity === 'error' ? 'bad' : 'warn'}>{t(`runs.issues.${issue.Severity}`)}</Badge>
              <span className="text-body-sm">{t('runs.issues.where', { path: issue.Path, line: issue.Line, column: issue.Column })}</span>
            </span>
            <span className="text-body-sm text-secondary">
              {isKnownIssue(issue.Code) ? t(`runs.issue.${issue.Code}`) : t('runs.issue.unknown', { message: issue.Message })}
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
};

const RunDetailView = ({ detail, props }: { detail: TRunDetail; props: IRunBodyProps }) => {
  const t = useT();
  const text = useRunText();
  const { Run: run } = detail;
  const final = isFinalRun(run.State);
  const now = useNow(!final);
  const reason = run.FailureReason === undefined || run.FailureReason === '' || run.State === 'succeeded' ? null : text.reason(run.FailureReason);

  return (
    <>
      <Card padded>
        <div className="grid gap-4">
          <div className="flex flex-wrap items-center gap-3">
            <StatusDot status={runStatusOf(run.State)}>{t(`runs.state.${run.State}`)}</StatusDot>
            {run.IsFork ? (
              <Badge tone="warn" icon="alert">
                {t('runs.fork')}
              </Badge>
            ) : null}
          </div>
          {run.State === 'awaiting_approval' ? <InlineAlert accent="awaiting" title={t('runs.approval.title')}>{t('runs.approval.body')}</InlineAlert> : null}
          {reason === null ? null : <InlineAlert accent={run.State === 'failed' ? 'failed' : 'queued'} title={reason} />}
          <RunFacts detail={detail} now={now} />
          <RunActions organizationId={props.organizationId} projectId={props.projectId} run={run} role={props.role} linked={props.linked} onSecondFactor={props.onSecondFactor} />
        </div>
      </Card>
      <Issues detail={detail} />
      <RunStages jobs={detail.Jobs} now={now} jobHref={props.jobHref} onOpenJob={props.onOpenJob} />
      {canOnProject(props.role, 'artifact.read') ? (
        <ArtifactsCard organizationId={props.organizationId} projectId={props.projectId} runId={run.Id} final={final} />
      ) : null}
    </>
  );
};

export const RunBody = (props: IRunBodyProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const run = useRun(props.organizationId, props.projectId, props.runId);
  useCrumbTitle(props.crumbKey, run.data === undefined ? undefined : t('runs.row.number', { number: run.data.Run.Number }));

  if (isApiError(run.error) && run.error.Status === 404) {
    return (
      <Card>
        <EmptyState icon="play" text={t('runs.detail.notFound')} />
      </Card>
    );
  }
  if (run.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('runs.detail.error')}>
          {errorText(run.error)}
        </InlineAlert>
        <Button size="sm" onClick={() => void run.refetch()}>
          {t('projects.retry')}
        </Button>
      </div>
    );
  }
  if (run.data === undefined) return <Skeleton label={t('runs.detail.loading')} lines={6} />;
  return <RunDetailView detail={run.data} props={props} />;
};

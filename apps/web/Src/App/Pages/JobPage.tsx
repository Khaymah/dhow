import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { isUuid } from '@/App/Pages/Params';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import { JobBody } from '@/Modules/Jobs/Components/JobBody';
import { WhyWaitingCard } from '@/Modules/Runners/Components/WhyWaitingCard';

export const JobPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { jobId } = useParams({ strict: false });
  if (!isUuid(jobId)) return <NotFoundPage />;

  return (
    <Page title={t('jobs.detail.title')} lead={t('jobs.detail.subtitle')}>
      <JobBody
        key={jobId}
        jobId={jobId}
        origin={(run) => (
          <p className="m-0 flex flex-wrap items-center gap-x-1.5 text-body-sm text-secondary">
            {t('jobs.origin.from')}
            <a
              href={`/projects/${run.ProjectId}/runs/${run.RunId}`}
              className="inline-flex min-h-11 items-center text-primary underline decoration-line-2 underline-offset-4 md:min-h-0"
              onClick={followInApp(() => {
                void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId: run.ProjectId, runId: run.RunId } });
              })}
            >
              {t('runs.row.number', { number: run.RunNumber })}
            </a>
            {t('jobs.origin.in')}
            <a
              href={`/projects/${run.ProjectId}`}
              className="inline-flex min-h-11 items-center text-primary underline decoration-line-2 underline-offset-4 md:min-h-0"
              onClick={followInApp(() => {
                void navigate({ to: '/projects/$projectId', params: { projectId: run.ProjectId } });
              })}
            >
              <bdi>{run.ProjectName}</bdi>
            </a>
          </p>
        )}
        waitingFor={(job) => (
          <WhyWaitingCard
            needs={{ Labels: job.Labels, Isolation: job.Isolation }}
            onAddRunner={() => {
              void navigate({ to: '/runners/tokens' });
            }}
            onOpenRunner={(runnerId) => {
              void navigate({ to: '/runners/$runnerId', params: { runnerId } });
            }}
          />
        )}
      />
    </Page>
  );
};

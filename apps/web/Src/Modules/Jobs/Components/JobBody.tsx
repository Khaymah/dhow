import { useQueryClient } from '@tanstack/react-query';
import { useCallback, type ReactNode } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { useCrumbTitle } from '@/Core/Hooks/useCrumbTitle';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TJob, TJobRun } from '@/Modules/Jobs/Api/JobsApi';
import { JobAttempts } from '@/Modules/Jobs/Components/JobAttempts';
import { JobLogSection } from '@/Modules/Jobs/Components/JobLogSection';
import { JobOverview } from '@/Modules/Jobs/Components/JobOverview';
import { JobSpecCard } from '@/Modules/Jobs/Components/JobSpecCard';
import { jobKey, useJob } from '@/Modules/Jobs/Hooks/useJobs';

export interface IJobBodyProps {
  jobId: string;
  waitingFor?: (job: TJob) => ReactNode;
  origin?: (run: TJobRun) => ReactNode;
}

const JobContent = ({ organizationId, jobId, waitingFor, origin }: IJobBodyProps & { organizationId: string }) => {
  const t = useT();
  const client = useQueryClient();
  const role = useCurrentMembership()?.Role;
  const job = useJob(organizationId, jobId);
  const name = job.data?.Run?.Kind === 'discovery' ? t('runs.job.system.discovery') : job.data?.Job.Name;
  useCrumbTitle(jobId, name);
  const HandleEnded = useCallback(() => {
    void client.invalidateQueries({ queryKey: jobKey(organizationId, jobId) });
  }, [client, organizationId, jobId]);

  if (job.isError) {
    if (isApiError(job.error) && job.error.Status === 404) {
      return (
        <Card>
          <EmptyState icon="play" text={t('jobs.detail.notFound')} />
        </Card>
      );
    }
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('jobs.detail.error')} />
        <Button
          size="sm"
          onClick={() => {
            void job.refetch();
          }}
        >
          {t('jobs.retry')}
        </Button>
      </div>
    );
  }
  if (job.data === undefined) return <Skeleton label={t('jobs.detail.loading')} lines={6} />;
  const detail = job.data;
  return (
    <>
      <JobOverview
        organizationId={organizationId}
        job={detail.Job}
        role={role}
        name={name}
        origin={detail.Run === undefined ? undefined : origin?.(detail.Run)}
        waitingFor={waitingFor}
      />
      <JobLogSection organizationId={organizationId} detail={detail} role={role} onEnded={HandleEnded} />
      <JobAttempts attempts={detail.Attempts} maxRetries={detail.Job.MaxInfraRetries} />
      <JobSpecCard spec={detail.Spec} />
    </>
  );
};

export const JobBody = ({ jobId, waitingFor, origin }: IJobBodyProps) => {
  const membership = useCurrentMembership();
  if (membership === undefined) return null;
  return <JobContent organizationId={membership.OrganizationId} jobId={jobId} waitingFor={waitingFor} origin={origin} />;
};

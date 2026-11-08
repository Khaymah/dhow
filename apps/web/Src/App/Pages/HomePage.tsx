import { useNavigate } from '@tanstack/react-router';

import { Page } from '@/App/Layout/Page';
import { jobsSearchOf } from '@/App/Navigation/JobsSearch';
import { canManageRunners, canReadAudit, canReadRunners, type TMembership } from '@/Core/Auth/AuthTypes';
import { useCurrentMembership, useMe } from '@/Core/Auth/useMe';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { MyApprovalsCard } from '@/Modules/Approvals/Components/MyApprovalsCard';
import { RecentEventsCard } from '@/Modules/Audit/Components/RecentEventsCard';
import type { TJobState } from '@/Modules/Jobs/Api/JobsApi';
import { WhatsNewCard } from '@/Modules/Help/Components/WhatsNewCard';
import { RecentJobsCard } from '@/Modules/Jobs/Components/RecentJobsCard';
import { AttentionCard } from '@/Modules/Overview/Components/AttentionCard';
import { OverviewStats } from '@/Modules/Overview/Components/OverviewStats';
import { StuckBanner } from '@/Modules/Overview/Components/StuckBanner';
import { useOverview } from '@/Modules/Overview/Hooks/useOverview';
import type { IOverviewLinks, TOverviewTarget } from '@/Modules/Overview/Types/OverviewTargets';
import { RunnersSummaryCard } from '@/Modules/Runners/Components/RunnersSummaryCard';
import { ServerStatusCard } from '@/Modules/System/Components/ServerStatusCard';
import { useReadiness } from '@/Modules/System/Hooks/useReadiness';

type TTargetPath = '/approvals' | '/jobs' | '/projects' | '/runners' | '/runners/tokens' | '/org/invitations' | '/org/settings' | '/system';

const TARGETS: Record<TOverviewTarget, { to: TTargetPath; states?: readonly TJobState[] }> = {
  running: { to: '/jobs', states: ['running'] },
  queued: { to: '/jobs', states: ['queued'] },
  stuck: { to: '/jobs', states: ['queued'] },
  failed: { to: '/jobs', states: ['failed', 'timed_out'] },
  runners: { to: '/runners' },
  offline: { to: '/runners' },
  invitations: { to: '/org/invitations' },
  approvals: { to: '/approvals' },
  checks: { to: '/system' },
  addRunner: { to: '/runners/tokens' },
  deploys: { to: '/projects' },
  rollbackFailed: { to: '/projects' },
  failedDeploys: { to: '/projects' },
  deadNotifications: { to: '/org/settings' },
};

const useLinks = (): IOverviewLinks => {
  const navigate = useNavigate();
  return {
    href: (target) => {
      const { to, states } = TARGETS[target];
      const { state } = jobsSearchOf(states ?? []);
      return state === undefined ? to : `${to}?state=${state}`;
    },
    open: (target) => {
      const { to, states } = TARGETS[target];
      void navigate(states === undefined ? { to } : { to: '/jobs', search: jobsSearchOf(states) });
    },
  };
};

const HomeStats = ({ organizationId, links }: { organizationId: string; links: IOverviewLinks }) => {
  const t = useT();
  const errorText = useApiErrorText();
  const overview = useOverview(organizationId);
  if (overview.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('home.overview.error')}>
          {errorText(overview.error)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void overview.refetch();
          }}
        >
          {t('jobs.retry')}
        </Button>
      </div>
    );
  }
  if (overview.data === undefined) return <Skeleton label={t('home.overview.loading')} lines={3} />;
  return <OverviewStats overview={overview.data} links={links} />;
};

const HomeContent = ({ membership }: { membership: TMembership }) => {
  const navigate = useNavigate();
  const links = useLinks();
  const organizationId = membership.OrganizationId;
  const role = membership.Role;
  const overview = useOverview(organizationId);
  const readiness = useReadiness();
  const failingChecks = readiness.data?.Checks.filter((check) => !check.Ok).length ?? 0;

  return (
    <>
      <StuckBanner jobs={overview.data?.Jobs ?? null} canAddRunner={canManageRunners(role)} links={links} />
      <HomeStats organizationId={organizationId} links={links} />
      {(overview.data?.Approvals?.Pending ?? 0) > 0 ? (
        <MyApprovalsCard
          organizationId={organizationId}
          hideWhenEmpty
          hrefFor={(approval) => `/projects/${approval.ProjectId}/runs/${approval.RunId}`}
          onOpen={(approval) => {
            void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId: approval.ProjectId, runId: approval.RunId } });
          }}
        />
      ) : null}
      <div className="grid gap-block lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)] lg:items-start">
        <div className="contents lg:grid lg:gap-block">
          <RecentJobsCard
            className="order-2"
            organizationId={organizationId}
            onOpenJob={(jobId) => {
              void navigate({ to: '/jobs/$jobId', params: { jobId } });
            }}
            onShowAll={() => {
              void navigate({ to: '/jobs' });
            }}
          />
          {canReadAudit(role) ? (
            <RecentEventsCard
              className="order-4"
              organizationId={organizationId}
              onOpenEvent={(eventId) => {
                void navigate({ to: '/audit/$eventId', params: { eventId } });
              }}
              onShowAll={() => {
                void navigate({ to: '/audit' });
              }}
            />
          ) : null}
        </div>
        <div className="contents lg:grid lg:gap-block">
          <AttentionCard className="order-1" overview={overview} failingChecks={failingChecks} links={links} />
          {canReadRunners(role) ? (
            <RunnersSummaryCard
              className="order-3"
              organizationId={organizationId}
              onOpenRunner={(runnerId) => {
                void navigate({ to: '/runners/$runnerId', params: { runnerId } });
              }}
              onShowAll={() => {
                void navigate({ to: '/runners' });
              }}
              onAddRunner={
                canManageRunners(role)
                  ? () => {
                      links.open('addRunner');
                    }
                  : undefined
              }
            />
          ) : null}
          <ServerStatusCard
            className="order-5"
            onOpenCheck={(check) => {
              void navigate({ to: '/system/checks/$check', params: { check } });
            }}
            onOpenStatus={() => {
              void navigate({ to: '/system' });
            }}
          />
        </div>
      </div>
    </>
  );
};

export const HomePage = () => {
  const t = useT();
  const me = useMe();
  const membership = useCurrentMembership();
  const homeless = me.data?.Memberships.length === 0;
  const navigate = useNavigate();

  return (
    <Page
      title={t('home.title')}
      lead={membership ? t('home.lead', { organization: membership.OrganizationName }) : undefined}
    >
      <WhatsNewCard
        onOpenAll={() => {
          void navigate({ to: '/help' });
        }}
      />
      {homeless ? (
        <Card title={t('organizations.none.title')} icon="building">
          <EmptyState icon="building" text={t('organizations.none.body')} />
        </Card>
      ) : null}
      {membership ? <HomeContent membership={membership} /> : null}
    </Page>
  );
};

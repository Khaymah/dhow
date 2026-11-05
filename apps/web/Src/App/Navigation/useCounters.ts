import type { TSectionId } from '@/App/Navigation/Sections';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import type { IMenuItem } from '@/Core/Ui/SideMenu';
import { useT } from '@/I18n/useT';
import { useOverview } from '@/Modules/Overview/Hooks/useOverview';

export type TCounter = Pick<IMenuItem, 'count' | 'countLabel' | 'countTone'>;

export interface ICounters {
  menu: Partial<Record<TSectionId, TCounter>>;
  jobsDot?: string;
}

export const useCounters = (): ICounters => {
  const t = useT();
  const membership = useCurrentMembership();
  const overview = useOverview(membership?.OrganizationId).data;
  const menu: Partial<Record<TSectionId, TCounter>> = {};
  const jobs = overview?.Jobs;
  const runners = overview?.Runners;
  const invitations = overview?.Invitations;
  const approvals = overview?.Approvals;

  if (jobs && jobs.Queued > 0) {
    const stuck = jobs.QueuedNoRunner > 0;
    menu.jobs = {
      count: jobs.Queued,
      countLabel: stuck
        ? t('shell.count.jobsStuck', { count: jobs.Queued, stuck: jobs.QueuedNoRunner })
        : t('shell.count.jobs', { count: jobs.Queued }),
      countTone: stuck ? 'warn' : undefined,
    };
  }
  if (approvals && approvals.Pending > 0) {
    menu.approvals = { count: approvals.Pending, countLabel: t('shell.count.approvals', { count: approvals.Pending }) };
  }
  if (runners && runners.Offline > 0) {
    menu.runners = { count: runners.Offline, countLabel: t('shell.count.runners', { count: runners.Offline }) };
  }
  if (invitations && invitations.Pending > 0) {
    menu.invitations = { count: invitations.Pending, countLabel: t('shell.count.invitations', { count: invitations.Pending }) };
  }

  return {
    menu,
    jobsDot: jobs && jobs.QueuedNoRunner > 0 ? t('shell.count.stuckDot', { count: jobs.QueuedNoRunner }) : undefined,
  };
};

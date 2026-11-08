export type TOverviewTarget = 'running' | 'queued' | 'stuck' | 'failed' | 'runners' | 'offline' | 'invitations' | 'approvals' | 'checks' | 'addRunner' | 'deploys' | 'rollbackFailed' | 'failedDeploys' | 'deadNotifications';

export interface IOverviewLinks {
  href: (target: TOverviewTarget) => string;
  open: (target: TOverviewTarget) => void;
}

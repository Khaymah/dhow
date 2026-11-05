export type TOverviewTarget = 'running' | 'queued' | 'stuck' | 'failed' | 'runners' | 'offline' | 'invitations' | 'approvals' | 'checks' | 'addRunner';

export interface IOverviewLinks {
  href: (target: TOverviewTarget) => string;
  open: (target: TOverviewTarget) => void;
}

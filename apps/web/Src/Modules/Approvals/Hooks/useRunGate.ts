import { canApproveDeployments, type TRole } from '@/Core/Auth/AuthTypes';
import { useMyApprovals, useRunApprovals } from '@/Modules/Approvals/Hooks/useApprovals';
import type { TApproval } from '@/Modules/Approvals/Api/ApprovalsApi';
import { hasPendingGate, needsApprovals, type TGateJob } from '@/Modules/Approvals/Types/Gate';

export interface IRunGateInput {
  organizationId: string;
  projectId: string;
  runId: string;
  jobs: readonly TGateJob[];
  role: TRole;
  live: boolean;
}

export interface IRunGate {
  Approvals: TApproval[] | undefined;
  Loading: boolean;
  Failed: boolean;
  Retry: () => void;
  CanDecide: (approval: TApproval) => boolean;
}

export const useRunGate = ({ organizationId, projectId, runId, jobs, role, live }: IRunGateInput): IRunGate => {
  const approvals = useRunApprovals(organizationId, projectId, runId, needsApprovals(jobs), live);
  const reviewer = canApproveDeployments(role);
  const mine = useMyApprovals(organizationId, reviewer && hasPendingGate(jobs));
  const ids = mine.data === undefined ? null : new Set(mine.data.map((approval) => approval.Id));

  return {
    Approvals: approvals.data,
    Loading: approvals.isPending && approvals.fetchStatus !== 'idle',
    Failed: approvals.isError,
    Retry: () => {
      void approvals.refetch();
    },
    CanDecide: (approval) => reviewer && (ids === null ? mine.isError : ids.has(approval.Id)),
  };
};

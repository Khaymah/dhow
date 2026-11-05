import type { components } from '@/Core/Api/Schema';
import type { TApproval } from '@/Modules/Approvals/Api/ApprovalsApi';

export type TGateJob = components['schemas']['RunJob'];
export type TDecision = 'approve' | 'reject' | 'bypass';
export type TGateKind = 'skipped' | 'approval' | 'timer' | 'environment_busy' | 'paused';

export const REF_SKIP_REASON = 'ENVIRONMENT_REF_NOT_ALLOWED';

export const gateKindOf = (job: TGateJob): TGateKind | null => {
  if (job.State === 'skipped' && job.FailureReason === REF_SKIP_REASON) return 'skipped';
  if (job.State === 'waiting' && job.Gate !== undefined) return job.Gate;
  return null;
};

export const needsApprovals = (jobs: readonly TGateJob[]): boolean => jobs.some((job) => job.Environment !== undefined && job.Environment !== '');

export const pendingFor = (approvals: readonly TApproval[] | undefined, job: TGateJob): TApproval | undefined =>
  approvals
    ?.filter((approval) => approval.State === 'pending' && approval.EnvironmentId === job.EnvironmentId)
    .sort((a, b) => b.Round - a.Round)[0];

export const isLeader = (jobs: readonly TGateJob[], job: TGateJob): boolean =>
  jobs.find((other) => gateKindOf(other) === 'approval' && other.EnvironmentId === job.EnvironmentId)?.Id === job.Id;

export const hasPendingGate =(jobs: readonly TGateJob[]): boolean => jobs.some((job) => gateKindOf(job) === 'approval');

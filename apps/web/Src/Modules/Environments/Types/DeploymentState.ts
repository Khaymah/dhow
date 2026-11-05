import type { TStatus } from '@/Core/Ui/StatusDot';
import type { TDeploymentState } from '@/Modules/Environments/Api/EnvironmentsApi';

const STATUS: Record<TDeploymentState, TStatus> = {
  queued: 'queued',
  running: 'building',
  succeeded: 'live',
  failed: 'failed',
  canceled: 'rolledback',
};

export const deploymentStatusOf = (state: TDeploymentState): TStatus => STATUS[state];

export const shortRef = (ref: string): string => ref.replace(/^refs\/(heads|tags)\//, '');

export const shortSha = (sha: string | undefined): string => (sha === undefined ? '' : sha.slice(0, 7));

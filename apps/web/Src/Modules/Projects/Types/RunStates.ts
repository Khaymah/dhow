import type { TStatus } from '@/Core/Ui/StatusDot';
import type { TRun, TRunJobState, TRunState } from '@/Modules/Projects/Api/RunsApi';

export const RUN_STATES: readonly TRunState[] = [
  'awaiting_approval',
  'pending',
  'queued',
  'running',
  'succeeded',
  'failed',
  'canceled',
  'skipped',
];

export type TRunFilter = 'all' | 'active' | 'failed' | 'succeeded' | 'awaiting';

export const RUN_FILTERS: readonly TRunFilter[] = ['all', 'active', 'awaiting', 'failed', 'succeeded'];

const FILTER_STATES: Record<TRunFilter, readonly TRunState[]> = {
  all: [],
  active: ['pending', 'queued', 'running'],
  awaiting: ['awaiting_approval'],
  failed: ['failed', 'canceled'],
  succeeded: ['succeeded'],
};

const FINAL: readonly TRunState[] = ['succeeded', 'failed', 'canceled', 'skipped'];
const RERUNNABLE: readonly TRunState[] = ['failed', 'canceled'];

const RUN_STATUS: Record<TRunState, TStatus> = {
  awaiting_approval: 'awaiting',
  pending: 'queued',
  queued: 'queued',
  running: 'building',
  succeeded: 'live',
  failed: 'failed',
  canceled: 'rolledback',
  skipped: 'rolledback',
};

const JOB_STATUS: Record<TRunJobState, TStatus> = {
  waiting: 'queued',
  active: 'building',
  succeeded: 'live',
  failed: 'failed',
  canceled: 'rolledback',
  skipped: 'rolledback',
};

export const statesOf = (filter: TRunFilter): readonly TRunState[] => FILTER_STATES[filter];

export const isFinalRun = (state: TRunState): boolean => FINAL.includes(state);

export const isRerunnable = (state: TRunState): boolean => RERUNNABLE.includes(state);

export const isCancelableRun = (run: Pick<TRun, 'State' | 'CancelRequestedAt'>): boolean =>
  !isFinalRun(run.State) && run.CancelRequestedAt === undefined;

export const runStatusOf = (state: TRunState): TStatus => RUN_STATUS[state];

export const jobStatusOf = (state: TRunJobState): TStatus => JOB_STATUS[state];

export const shortSha = (sha: string | undefined): string => (sha === undefined ? '' : sha.slice(0, 7));

export const shortRef = (ref: string): string => ref.replace(/^refs\/(heads|tags)\//, '');

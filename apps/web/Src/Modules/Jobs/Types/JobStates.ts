import type { TRole } from '@/Core/Auth/AuthTypes';
import type { TStatus } from '@/Core/Ui/StatusDot';
import type { TLocale } from '@/I18n/Locale';
import type { TJob, TJobAttemptState, TJobState } from '@/Modules/Jobs/Api/JobsApi';

export const JOB_STATES: readonly TJobState[] = [
  'queued',
  'leased',
  'running',
  'succeeded',
  'failed',
  'canceled',
  'timed_out',
  'lost',
];

const FINAL_STATES: readonly string[] = ['succeeded', 'failed', 'canceled', 'timed_out', 'lost'];

const STATUS: Record<TJobState, TStatus> = {
  queued: 'queued',
  leased: 'building',
  running: 'building',
  succeeded: 'live',
  failed: 'failed',
  canceled: 'rolledback',
  timed_out: 'failed',
  lost: 'awaiting',
};

export const NO_MATCHING_RUNNER = 'NO_MATCHING_RUNNER';

export const JOBS_GUIDE_URL: Record<TLocale, string> = {
  ar: 'https://github.com/Khaymah/dhow/blob/main/docs/guides/jobs.md#إنشاء-تشغيل-عبر-api',
  en: 'https://github.com/Khaymah/dhow/blob/main/docs/en/guides/jobs.md#create-a-job-through-the-api',
};

export const FAILURE_REASONS = [
  'CANCELED',
  'QUEUE_TIMEOUT',
  'LOST_AFTER_MUTATION',
  'DEPLOY_INTERRUPTED',
  'RETRIES_EXHAUSTED',
  'LEASE_LOST',
  'RUNNER_REVOKED',
  'RUNNER_RESTARTED',
  'TIMED_OUT',
  'START_TIMEOUT',
  'SECRETS_UNAVAILABLE',
  'IMAGE_PULL_FAILED',
  'OOM_KILLED',
  'PIDS_LIMIT_REACHED',
  'RESOURCE_REQUEST_EXCEEDS_RUNNER',
  'RUNNER_DISK_FULL',
  'SANDBOX_FAILED',
  'SHELL_NOT_FOUND',
  'SPEC_REJECTED',
  'STEP_FAILED',
  'NO_MATCHING_RUNNER',
] as const;

export type TFailureReason = (typeof FAILURE_REASONS)[number];

const CANCEL_ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer'];
const LOG_ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer'];
const CREATE_ROLES: readonly TRole[] = ['owner', 'admin'];

export const isFinalState = (state: TJobState | TJobAttemptState): boolean => FINAL_STATES.includes(state);

export const statusOf = (job: Pick<TJob, 'State' | 'QueuedReason'>): TStatus =>
  job.State === 'queued' && job.QueuedReason === NO_MATCHING_RUNNER ? 'awaiting' : STATUS[job.State];

export const attemptStatusOf = (state: TJobAttemptState): TStatus => STATUS[state];

export const isKnownReason = (reason: string): reason is TFailureReason =>
  (FAILURE_REASONS as readonly string[]).includes(reason);

export const isStuck = (job: Pick<TJob, 'State' | 'QueuedReason'>): boolean =>
  job.State === 'queued' && job.QueuedReason === NO_MATCHING_RUNNER;

export const canCancelJobs = (role: TRole | undefined): boolean => role !== undefined && CANCEL_ROLES.includes(role);

export const canReadJobLogs = (role: TRole | undefined): boolean => role !== undefined && LOG_ROLES.includes(role);

export const canCreateJobs = (role: TRole | undefined): boolean => role !== undefined && CREATE_ROLES.includes(role);

export const isCancelable = (job: Pick<TJob, 'State' | 'CancelRequestedAt'>): boolean =>
  !isFinalState(job.State) && job.CancelRequestedAt === undefined;

import type { TBadgeTone } from '@/Core/Ui/Badge';
import type { TStatus } from '@/Core/Ui/StatusDot';
import type { THostCheck, TReleaseHost, TTargetCheck, TTargetRelease, TTargetReleases } from '@/Modules/Environments/Api/TargetsApi';

export const CHECK_POLL_MS = 4000;
export const CHECK_WATCH_MS = 5 * 60 * 1000;

type TCheckReason = NonNullable<THostCheck['Reason']>;

export const CHECK_REASONS: readonly TCheckReason[] = [
  'ok',
  'host_key_mismatch',
  'unreachable',
  'auth_failed',
  'path_missing',
  'current_not_symlink',
  'state_corrupt',
  'disk_low',
  'locked',
  'diverged',
];

export const checkTone = (reason: TCheckReason): TBadgeTone => {
  if (reason === 'ok') return 'ok';
  if (reason === 'diverged' || reason === 'locked' || reason === 'disk_low') return 'warn';
  return 'bad';
};

export const releaseStatusOf = (state: TTargetRelease['State']): TStatus => {
  switch (state) {
    case 'succeeded':
      return 'live';
    case 'running':
      return 'building';
    case 'rolled_back':
      return 'rolledback';
    case 'unknown':
      return 'awaiting';
    default:
      return 'failed';
  }
};

export const hostStatusOf = (state: TReleaseHost['State']): TStatus => {
  switch (state) {
    case 'verified':
      return 'live';
    case 'pending':
    case 'uploaded':
    case 'switched':
      return 'building';
    case 'rolled_back':
    case 'skipped':
      return 'rolledback';
    case 'unknown':
      return 'awaiting';
    default:
      return 'failed';
  }
};

export const checkedSince = (check: TTargetCheck | undefined, since: number): boolean =>
  check?.Hosts.every((host) => host.CheckedAt !== undefined && Date.parse(host.CheckedAt) >= since) === true;

export const checkIsPending = (check: TTargetCheck | undefined, since: number | null, now: number): boolean =>
  since !== null && now - since <= CHECK_WATCH_MS && !checkedSince(check, since);

export const differentReleases = (releases: TTargetReleases | undefined, check: TTargetCheck | undefined): boolean => {
  if (releases?.Partial === true) return true;
  const current = new Set((check?.Hosts ?? []).map((host) => host.CurrentReleaseId).filter((id): id is string => id !== undefined && id !== ''));
  return current.size > 1;
};

export const canRollBackTo = (release: TTargetRelease, current: string | undefined, partial: boolean): boolean =>
  release.RollbackTarget && (release.ReleaseId !== current || partial);

export const ROLLBACK_REASONS = [
  'health_check_failed',
  'reload_failed',
  'hook_failed',
  'canceled',
  'upload_failed',
  'switch_failed',
  'other',
] as const;

export type TRollbackReason = (typeof ROLLBACK_REASONS)[number];

export const rollbackReasonOf = (reason: string | undefined): TRollbackReason | null => {
  if (reason === undefined || reason === '') return null;
  return ROLLBACK_REASONS.find((known) => known === reason) ?? 'other';
};

export const isAutomaticRollback = (release: TTargetRelease): boolean => release.Kind === 'deploy' && release.State === 'rolled_back';

export interface ILastCheck {
  At: string;
  Problems: number;
  Hosts: number;
}

export const lastCheckOf = (check: TTargetCheck | undefined): ILastCheck | null => {
  const checked = (check?.Hosts ?? []).filter((host) => host.Checked && host.CheckedAt !== undefined);
  if (checked.length === 0) return null;
  const times = checked.map((host) => Date.parse(host.CheckedAt ?? ''));
  const latest = checked[times.indexOf(Math.max(...times))]?.CheckedAt;
  if (latest === undefined) return null;
  return { At: latest, Problems: checked.filter((host) => host.Reason !== 'ok').length, Hosts: check?.Hosts.length ?? checked.length };
};

const RELEASE_STATES = ['running', 'succeeded', 'failed', 'rolled_back', 'rollback_failed', 'unknown'] as const;
const HOST_STATES = ['pending', 'uploaded', 'switched', 'verified', 'failed', 'rolled_back', 'rollback_failed', 'unknown', 'skipped'] as const;
const RELEASE_KINDS = ['deploy', 'rollback'] as const;

export const releaseStateKey = (state: string): `targets.release.state.${(typeof RELEASE_STATES)[number]}` =>
  `targets.release.state.${RELEASE_STATES.find((known) => known === state) ?? 'unknown'}`;

export const hostStateKey = (state: string): `targets.release.host.${(typeof HOST_STATES)[number]}` =>
  `targets.release.host.${HOST_STATES.find((known) => known === state) ?? 'unknown'}`;

export const releaseKindKey = (kind: string): `targets.release.kind.${(typeof RELEASE_KINDS)[number]}` =>
  `targets.release.kind.${RELEASE_KINDS.find((known) => known === kind) ?? 'deploy'}`;

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

export const formatSize = (bytes: number): string => {
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? String(Math.round(value)) : value.toFixed(1)} ${UNITS[unit] ?? 'B'}`;
};

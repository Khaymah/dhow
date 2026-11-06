import { describe, expect, it } from 'vitest';

import { releaseFixture } from '@/Core/Testing/TargetsBackend';
import type { TTargetCheck, TTargetRelease } from '@/Modules/Environments/Api/TargetsApi';
import { hostStateKey, isAutomaticRollback, lastCheckOf, releaseKindKey, releaseStateKey, rollbackReasonOf } from '@/Modules/Environments/Types/TargetState';

const release = (over: Record<string, unknown>) => releaseFixture('12-aaaaaaaa-1', over) as unknown as TTargetRelease;

describe('an automatic rollback', () => {
  it('is a deploy that Dhow rolled back', () => {
    expect(isAutomaticRollback(release({ State: 'rolled_back' }))).toBe(true);
  });

  it('is not a manual rollback, a success or a rollback that failed', () => {
    expect(isAutomaticRollback(release({ Kind: 'rollback', State: 'rolled_back' }))).toBe(false);
    expect(isAutomaticRollback(release({ State: 'succeeded' }))).toBe(false);
    expect(isAutomaticRollback(release({ State: 'rollback_failed' }))).toBe(false);
  });
});

describe('the reason of a rollback', () => {
  it('keeps the reasons the engine reports', () => {
    expect(rollbackReasonOf('health_check_failed')).toBe('health_check_failed');
    expect(rollbackReasonOf('canceled')).toBe('canceled');
  });

  it('files an unknown reason under other and says nothing for no reason', () => {
    expect(rollbackReasonOf('something new')).toBe('other');
    expect(rollbackReasonOf('')).toBeNull();
    expect(rollbackReasonOf(undefined)).toBeNull();
  });
});

describe('the last check', () => {
  const host = (position: number, over: Record<string, unknown> = {}) => ({
    HostId: `0198a000-0000-7000-8000-0000000007a${String(position)}`,
    Position: position,
    Host: `web-${String(position)}.example.com`,
    Port: 22,
    Checked: true,
    Reason: 'ok',
    Locked: false,
    Repaired: false,
    CheckedAt: '2026-11-05T08:00:00Z',
    ...over,
  });

  it('is nothing when no server was checked', () => {
    expect(lastCheckOf(undefined)).toBeNull();
    expect(lastCheckOf({ Hosts: [host(1, { Checked: false, CheckedAt: undefined })] } as unknown as TTargetCheck)).toBeNull();
  });

  it('takes the latest time and counts the servers with a problem', () => {
    const check = { Hosts: [host(1), host(2, { Reason: 'unreachable', CheckedAt: '2026-11-05T09:30:00Z' }), host(3, { Checked: false, CheckedAt: undefined })] };
    expect(lastCheckOf(check as unknown as TTargetCheck)).toEqual({ At: '2026-11-05T09:30:00Z', Problems: 1, Hosts: 3 });
  });
});

describe('the keys of the states and kinds', () => {
  it('keep the known values', () => {
    expect(releaseStateKey('rolled_back')).toBe('targets.release.state.rolled_back');
    expect(hostStateKey('skipped')).toBe('targets.release.host.skipped');
    expect(releaseKindKey('rollback')).toBe('targets.release.kind.rollback');
  });

  it('send an unknown value to a key that exists', () => {
    expect(releaseStateKey('future')).toBe('targets.release.state.unknown');
    expect(hostStateKey('future')).toBe('targets.release.host.unknown');
    expect(releaseKindKey('future')).toBe('targets.release.kind.deploy');
  });
});

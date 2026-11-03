import { describe, expect, it } from 'vitest';

import { canOnProject, canRunForksWithoutApproval } from '@/Modules/Projects/Types/ProjectAccess';
import { isKnownReason, REASONS } from '@/Modules/Projects/Types/Reasons';
import { isCancelableRun, isFinalRun, isRerunnable, RUN_STATES, shortRef, shortSha, statesOf } from '@/Modules/Projects/Types/RunStates';
import { Ar } from '@/I18n/Locales/Ar';
import { En } from '@/I18n/Locales/En';

describe('the run states', () => {
  it('knows which runs are over, can be canceled or run again', () => {
    expect(RUN_STATES.filter(isFinalRun)).toEqual(['succeeded', 'failed', 'canceled', 'skipped']);
    expect(RUN_STATES.filter(isRerunnable)).toEqual(['failed', 'canceled']);
    expect(isCancelableRun({ State: 'running' })).toBe(true);
    expect(isCancelableRun({ State: 'running', CancelRequestedAt: '2026-11-01T08:00:00Z' })).toBe(false);
    expect(isCancelableRun({ State: 'succeeded' })).toBe(false);
  });

  it('maps every filter to the states the API filters on', () => {
    expect(statesOf('all')).toEqual([]);
    expect(statesOf('active')).toEqual(['pending', 'queued', 'running']);
    expect(statesOf('failed')).toEqual(['failed', 'canceled']);
  });

  it('shortens refs and commits', () => {
    expect(shortRef('refs/heads/main')).toBe('main');
    expect(shortRef('refs/tags/v1')).toBe('v1');
    expect(shortRef('refs/pull/3/head')).toBe('refs/pull/3/head');
    expect(shortSha('0123456789abcdef')).toBe('0123456');
    expect(shortSha(undefined)).toBe('');
  });
});

describe('the failure reasons', () => {
  it('has a sentence in both languages for every known code, and never shows the code alone', () => {
    for (const code of REASONS) {
      expect(isKnownReason(code)).toBe(true);
      expect(Ar[`runs.reason.${code}`]).not.toContain(code);
      expect(En[`runs.reason.${code}`].length).toBeGreaterThan(code.length);
    }
    expect(isKnownReason('SOMETHING_NEW')).toBe(false);
    expect(Ar['runs.reason.unknown']).toContain('{code}');
  });
});

describe('the project roles', () => {
  it('mirror the server grants for project actions', () => {
    expect(canOnProject('viewer', 'run.trigger')).toBe(false);
    expect(canOnProject('developer', 'run.trigger')).toBe(true);
    expect(canOnProject('developer', 'run.cancel')).toBe(false);
    expect(canOnProject('developer', 'artifact.read')).toBe(true);
    expect(canOnProject('viewer', 'artifact.read')).toBe(false);
    expect(canOnProject('maintainer', 'run.approve')).toBe(true);
    expect(canOnProject('maintainer', 'project.manage')).toBe(true);
    expect(canOnProject(undefined, 'project.manage')).toBe(false);
    expect(canRunForksWithoutApproval('maintainer')).toBe(false);
    expect(canRunForksWithoutApproval('admin')).toBe(true);
  });
});

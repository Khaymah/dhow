import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { createQueryClient } from '@/Core/Api/QueryClient';
import { z } from '@/Core/Forms/Zod';
import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch } from '@/Core/Testing/Fetch';
import { detailFixture, notLinked, ok, page, PROJECT_ID, projectFixture, repositoryFixture, RUN_ID, runFixture } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { I18nProvider } from '@/I18n/I18nProvider';
import { Ar } from '@/I18n/Locales/Ar';
import { useIssueText } from '@/Modules/Projects/Hooks/useIssueText';
import { useNow } from '@/Modules/Projects/Hooks/useNow';
import { useOrganizationAccess } from '@/Modules/Projects/Hooks/useProjectAccess';
import { useArchiveProject, useProject, useProjects, useUpdateProject } from '@/Modules/Projects/Hooks/useProjects';
import { isNotLinked, useRepository, useUnlinkRepository } from '@/Modules/Projects/Hooks/useRepository';
import { useRunText } from '@/Modules/Projects/Hooks/useRunText';
import { DETAIL_REFRESH_MS, useArtifacts, useManualInputs, useRun, useRunAction, useRuns } from '@/Modules/Projects/Hooks/useRuns';

const wrapper = ({ children }: { children: ReactNode }) => (
  <I18nProvider>
    <QueryClientProvider client={createQueryClient({ retry: false })}>{children}</QueryClientProvider>
  </I18nProvider>
);

const path = (request: Request) => new URL(request.url).pathname;

afterEach(() => {
  vi.useRealTimers();
});

describe('the project hooks', () => {
  it('read the organization access from the session', async () => {
    stubFetch(() => envelope(meBody('developer')));
    const { result } = renderHook(() => useOrganizationAccess(), { wrapper });
    expect(result.current.Kind).toBe('loading');
    await waitFor(() => {
      expect(result.current).toEqual({ Kind: 'allowed', OrganizationId: ORG_ID, Role: 'developer' });
    });
  });

  it('list, read, change and archive projects', async () => {
    const methods: string[] = [];
    stubFetch(async (request) => {
      methods.push(`${request.method} ${path(request).split('/').pop() ?? ''}`);
      if (request.method === 'PATCH') return ok(projectFixture((await request.json()) as Record<string, unknown>));
      if (request.method === 'POST') return ok(projectFixture({ State: 'archived' }));
      return path(request).endsWith('/projects') ? page([projectFixture()]) : ok(projectFixture({ CallerRole: 'maintainer' }));
    });
    const { result } = renderHook(
      () => ({
        list: useProjects(ORG_ID, 1),
        one: useProject(ORG_ID, PROJECT_ID),
        none: useProject('', PROJECT_ID),
        update: useUpdateProject(ORG_ID, PROJECT_ID),
        archive: useArchiveProject(ORG_ID, PROJECT_ID),
      }),
      { wrapper },
    );
    await waitFor(() => {
      expect(result.current.one.data?.CallerRole).toBe('maintainer');
    });
    expect(result.current.list.data?.Items).toHaveLength(1);
    expect(result.current.none.fetchStatus).toBe('idle');
    await act(() => result.current.update.mutateAsync({ Name: 'Renamed' }));
    await waitFor(() => {
      expect(result.current.one.data?.Name).toBe('Renamed');
    });
    await act(() => result.current.archive.mutateAsync());
    expect(methods).toContain('PATCH ' + PROJECT_ID);
    expect(methods).toContain('POST archive');
  });

  it('treat a missing repository as not linked and forget it after an unlink', async () => {
    let linked = true;
    stubFetch((request) => {
      if (request.method === 'DELETE') {
        linked = false;
        return ok(repositoryFixture());
      }
      return linked ? ok(repositoryFixture()) : notLinked();
    });
    const { result } = renderHook(() => ({ repo: useRepository(ORG_ID, PROJECT_ID, true), unlink: useUnlinkRepository(ORG_ID, PROJECT_ID) }), { wrapper });
    await waitFor(() => {
      expect(result.current.repo.data?.Transport).toBe('ssh');
    });
    await act(() => result.current.unlink.mutateAsync());
    await waitFor(() => {
      expect(isNotLinked(result.current.repo.error)).toBe(true);
    });
    expect(result.current.repo.data).toBeUndefined();
    expect(isNotLinked(new Error('x'))).toBe(false);
  });
});

describe('the run hooks', () => {
  it('poll a run only while it is not over', async () => {
    let state = 'running';
    const { calls } = stubFetch(() => ok(detailFixture(runFixture({ State: state }))));
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { result } = renderHook(() => useRun(ORG_ID, PROJECT_ID, RUN_ID), { wrapper });
    await waitFor(() => {
      expect(result.current.data?.Run.State).toBe('running');
    });
    state = 'succeeded';
    await act(async () => {
      await vi.advanceTimersByTimeAsync(DETAIL_REFRESH_MS + 10);
    });
    await waitFor(() => {
      expect(result.current.data?.Run.State).toBe('succeeded');
    });
    const seen = calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(DETAIL_REFRESH_MS * 3);
    });
    expect(calls.length).toBe(seen);
  });

  it('list runs, artifacts and manual inputs, and run an action', async () => {
    stubFetch((request) => {
      const p = path(request);
      if (request.method === 'POST') return ok(detailFixture(runFixture({ State: 'canceled' })));
      if (p.endsWith('/runs')) return page([runFixture()]);
      if (p.endsWith('/artifacts')) return ok([]);
      if (p.endsWith('/manual-inputs')) return ok({ Known: false, Enabled: false, Ref: 'refs/heads/main', Inputs: [] });
      return backendFailure('NOT_FOUND', 'x', 404);
    });
    const { result } = renderHook(
      () => ({
        runs: useRuns(ORG_ID, PROJECT_ID, 1, ['failed']),
        artifacts: useArtifacts(ORG_ID, PROJECT_ID, RUN_ID, true),
        inputs: useManualInputs(ORG_ID, PROJECT_ID),
        off: useManualInputs(ORG_ID, PROJECT_ID, false),
        cancel: useRunAction(ORG_ID, PROJECT_ID, RUN_ID, 'cancel'),
      }),
      { wrapper },
    );
    await waitFor(() => {
      expect(result.current.runs.data?.Items).toHaveLength(1);
    });
    await waitFor(() => {
      expect(result.current.inputs.data?.Known).toBe(false);
    });
    expect(result.current.artifacts.data).toEqual([]);
    expect(result.current.off.fetchStatus).toBe('idle');
    const detail = await act(() => result.current.cancel.mutateAsync());
    expect(detail.Run.State).toBe('canceled');
  });
});

describe('the text hooks', () => {
  it('describe reasons, durations, actors and sizes', () => {
    const { result } = renderHook(() => useRunText(), { wrapper });
    const text = result.current;
    expect(text.reason('STEP_FAILED')).toBe(Ar['runs.reason.STEP_FAILED']);
    expect(text.reason('NEW')).toBe(arText('runs.reason.unknown', { code: 'NEW' }));
    expect(text.duration('2026-11-01T08:00:00Z', '2026-11-01T09:02:00Z')).toBe(arText('jobs.duration.hours', { h: 1, m: 2 }));
    expect(text.duration('bad', 0)).toBe('');
    expect(text.runDuration({ StartedAt: undefined }, 0)).toBe('');
    expect(text.actor({ Trigger: 'push', Actor: 'sara' })).toBe('sara');
    expect(text.actor({ Trigger: 'manual', TriggeredByUserId: ORG_ID })).toBe(Ar['runs.actor.member']);
    expect(text.actor({ Trigger: 'api' })).toBe(Ar['runs.actor.token']);
    expect(text.actor({ Trigger: 'push' })).toBe(Ar['runs.actor.unknown']);
    expect(text.size(512)).toBe(arText('jobs.size.bytes', { n: 512 }));
    expect(text.size(2048)).toBe(arText('runs.size.kib', { n: 2 }));
  });

  it('turn zod issues into translated messages, one per field', () => {
    const { result } = renderHook(() => useIssueText(), { wrapper });
    const parsed = z.object({ Name: z.string().min(1, { error: 'projects.form.error.name' }), Other: z.number() }).safeParse({ Name: '', Other: 'x' });
    if (parsed.success) throw new Error('expected a failure');
    const issues = result.current(parsed.error);
    expect(issues.Name).toBe(Ar['projects.form.error.name']);
    expect(issues.Other).toBe(Ar['projects.form.error.invalid']);
  });

  it('ticks the clock only while active', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { result, rerender } = renderHook(({ active }) => useNow(active, 1000), { initialProps: { active: true } });
    const first = result.current;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1100);
    });
    expect(result.current).toBeGreaterThan(first);
    rerender({ active: false });
    const stopped = result.current;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(result.current).toBe(stopped);
  });
});

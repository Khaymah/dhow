import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend } from '@/Core/Testing/Backend';
import { ENVIRONMENT_NAME, environmentFixture, environmentsRoute, newEnvironmentsState } from '@/Core/Testing/EnvironmentsBackend';
import { findPage } from '@/Core/Testing/Pages';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import {
  HOST_ONE,
  HOST_THREE,
  HOST_TWO,
  hostCheckFixture,
  newTargetsState,
  releaseFixture,
  targetFixture,
  targetsRoute,
  type ITargetsState,
} from '@/Core/Testing/TargetsBackend';
import { arText } from '@/Core/Testing/Text';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const PASSWORD = 'correct horse battery staple';
const PAGE_URL = `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`;
const API = `/organizations/${ORG_ID}/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}/targets/web`;

const mount = (role = 'owner', state: ITargetsState = newTargetsState(), paused = false) => {
  const environments = newEnvironmentsState();
  environments.environments = [paused ? environmentFixture({ Paused: true, PausedReason: 'Freeze' }) : environmentFixture()];
  const backend = startBackend({
    role,
    routes: [targetsRoute(state), environmentsRoute(environments), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [PAGE_URL] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

const targetsRegion = async () => {
  const main = await findPage(ENVIRONMENT_NAME);
  return within(main).findByRole('region', { name: Ar['targets.group'] });
};

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the targets of an environment', () => {
  it('shows the target, what is deployed now and the result of the last check of every host', async () => {
    const state = newTargetsState();
    state.check = {
      Hosts: [
        hostCheckFixture(HOST_ONE, 1),
        hostCheckFixture(HOST_TWO, 2, { Reason: 'disk_low', FreeBytes: 1024 * 1024 }),
        hostCheckFixture(HOST_THREE, 3, { Reason: 'unreachable', CurrentReleaseId: undefined, VerifiedReleaseId: undefined, FreeBytes: undefined }),
      ],
    };
    mount('owner', state);
    const region = await targetsRegion();

    expect(await within(region).findByRole('heading', { name: 'web' })).toBeInTheDocument();
    expect(await within(region).findByText(arText('targets.current', { release: '12-aaaaaaaa-1' }))).toBeInTheDocument();
    expect(await within(region).findByText(Ar['targets.check.reason.ok'])).toBeInTheDocument();
    expect(within(region).getByText(Ar['targets.check.reason.disk_low'])).toBeInTheDocument();
    expect(within(region).getByText(Ar['targets.check.reason.unreachable'])).toBeInTheDocument();
    expect(within(region).getByText(arText('targets.check.free', { size: '1.0 MB' }), { exact: false })).toBeInTheDocument();
    expect(within(region).queryByText(Ar['targets.partial'])).not.toBeInTheDocument();
  });

  it('says a host was never checked', async () => {
    mount();
    const region = await targetsRegion();
    expect(await within(region).findAllByText(Ar['targets.check.never'])).toHaveLength(3);
  });

  it('shows the empty state when the environment has no target', async () => {
    const state = newTargetsState();
    state.targets = [];
    mount('owner', state);
    const region = await targetsRegion();
    expect(await within(region).findByText(Ar['targets.empty'])).toBeInTheDocument();
  });

  it('badges a group whose last release left servers on different releases', async () => {
    const state = newTargetsState();
    state.releases = { ...state.releases, Partial: true };
    mount('owner', state);
    const region = await targetsRegion();
    expect((await within(region).findAllByText(Ar['targets.partial'])).length).toBeGreaterThan(0);
    expect(within(region).getByText(Ar['targets.partial.hint'])).toBeInTheDocument();
  });

  it('computes the badge from a check that found two releases, whatever the last release says', async () => {
    const state = newTargetsState();
    state.check = {
      Hosts: [
        hostCheckFixture(HOST_ONE, 1),
        hostCheckFixture(HOST_TWO, 2, { CurrentReleaseId: '11-bbbbbbbb-1', Reason: 'diverged' }),
        hostCheckFixture(HOST_THREE, 3),
      ],
    };
    mount('owner', state);
    const region = await targetsRegion();
    expect((await within(region).findAllByText(Ar['targets.partial'])).length).toBeGreaterThan(0);
  });

  it('says the target is not ready when a host key is missing', async () => {
    const state = newTargetsState();
    state.targets = [targetFixture({ Ready: false })];
    mount('owner', state);
    const region = await targetsRegion();
    expect(await within(region).findByText(Ar['targets.notReady'])).toBeInTheDocument();
  });
});

describe('checking a target', () => {
  it('starts the check, says so, and shows the new result of every host', async () => {
    const state = newTargetsState();
    state.checkAfter = { Hosts: [HOST_ONE, HOST_TWO, HOST_THREE].map((id, index) => hostCheckFixture(id, index + 1, { CheckedAt: '2999-01-01T00:00:00Z' })) };
    const view = mount('maintainer', state);
    const region = await targetsRegion();
    await view.user.click(await within(region).findByRole('button', { name: Ar['targets.check.button'] }));

    await waitFor(() => {
      expect(state.writes).toEqual([{ Method: 'POST', Path: `${API}/check`, Body: null }]);
    });
    expect(await screen.findByText(arText('targets.check.started', { name: 'web' }))).toBeInTheDocument();
    expect(await within(region).findAllByText(Ar['targets.check.reason.ok'])).toHaveLength(3);
  });

  it.each(['developer', 'viewer'] as const)('does not offer the check or the rollback to a %s', async (role) => {
    mount(role);
    const region = await targetsRegion();
    await within(region).findByRole('heading', { name: 'web' });
    expect(within(region).queryByRole('button', { name: Ar['targets.check.button'] })).not.toBeInTheDocument();
    await within(region).findAllByText(arText('targets.release.state.succeeded'));
    expect(within(region).queryByRole('button', { name: Ar['targets.rollback.action'] })).not.toBeInTheDocument();
    expect(within(region).getByText(Ar['targets.forbidden'])).toBeInTheDocument();
  });

  it('shows the failure of a check that could not start', async () => {
    const state = newTargetsState();
    state.failCheck = backendFailure('TARGET_CHECK_UNAVAILABLE', 'هذا الإصدار لا يحمل الصورة.', 422);
    const view = mount('owner', state);
    const region = await targetsRegion();
    await view.user.click(await within(region).findByRole('button', { name: Ar['targets.check.button'] }));
    expect(await within(region).findByText('هذا الإصدار لا يحمل الصورة.')).toBeInTheDocument();
  });
});

describe('rolling a target back', () => {
  const releaseRow = async (region: HTMLElement, releaseId: string): Promise<HTMLElement> => {
    const row = (await within(region).findByText(releaseId)).closest('li');
    if (!row) throw new Error(`no row for the release ${releaseId}`);
    return row;
  };

  it('asks for the second step and the confirmation, then rolls every server back', async () => {
    const state = newTargetsState();
    const view = mount('maintainer', state);
    const region = await targetsRegion();
    const row = await releaseRow(region, '11-bbbbbbbb-1');
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.rollback.action'] }));

    expect(within(region).getByText(arText('targets.rollback.question', { target: 'web', release: '11-bbbbbbbb-1' }))).toBeInTheDocument();
    expect(state.writes).toHaveLength(0);
    await view.user.type(await within(region).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(region).getByRole('button', { name: Ar['targets.rollback.confirm'] }));

    await waitFor(() => {
      expect(state.writes).toHaveLength(1);
    });
    expect(state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/rollback`,
      Body: { ReleaseId: '11-bbbbbbbb-1', CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await screen.findByText(arText('targets.rollback.started', { release: '11-bbbbbbbb-1' }))).toBeInTheDocument();
    expect(within(region).queryByText(Ar['targets.rollback.confirm'])).not.toBeInTheDocument();
  });

  it('does not offer the current release, a rollback, or a release that did not succeed', async () => {
    const state = newTargetsState();
    state.releases = {
      CurrentReleaseId: '12-aaaaaaaa-1',
      Partial: false,
      Releases: [
        releaseFixture('13-cccccccc-1', { Kind: 'rollback', RestoredReleaseId: '12-aaaaaaaa-1', RollbackTarget: false }),
        releaseFixture('12-aaaaaaaa-1'),
        releaseFixture('11-bbbbbbbb-1', { State: 'rolled_back', RollbackTarget: false }),
        releaseFixture('10-dddddddd-1'),
      ],
    };
    mount('owner', state);
    const region = await targetsRegion();
    await within(region).findByText('10-dddddddd-1');
    expect(within(region).getAllByRole('button', { name: Ar['targets.rollback.action'] })).toHaveLength(1);
    const only = (await releaseRow(region, '10-dddddddd-1')).querySelector('button');
    expect(only).not.toBeNull();
  });

  it('offers the current release again when the servers are on different releases', async () => {
    const state = newTargetsState();
    state.releases = { ...state.releases, Partial: true };
    mount('owner', state);
    const region = await targetsRegion();
    await within(region).findByText('12-aaaaaaaa-1');
    expect(within(region).getAllByRole('button', { name: Ar['targets.rollback.action'] })).toHaveLength(2);
  });

  it('warns that the artifact is gone and still lets the second step through', async () => {
    const state = newTargetsState();
    state.releases = {
      CurrentReleaseId: '12-aaaaaaaa-1',
      Partial: false,
      Releases: [releaseFixture('12-aaaaaaaa-1'), releaseFixture('11-bbbbbbbb-1', { ArtifactKept: false })],
    };
    const view = mount('admin', state);
    const region = await targetsRegion();
    const row = await releaseRow(region, '11-bbbbbbbb-1');
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.rollback.action'] }));
    expect(await within(region).findByText(Ar['targets.rollback.noArtifact'])).toBeInTheDocument();
  });

  it('keeps the panel open and says why when the server refuses the rollback', async () => {
    const state = newTargetsState();
    state.failRollback = backendFailure('ENVIRONMENT_PAUSED', 'البيئة متوقفة.', 409);
    const view = mount('maintainer', state);
    const region = await targetsRegion();
    const row = await releaseRow(region, '11-bbbbbbbb-1');
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.rollback.action'] }));
    await view.user.type(await within(region).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(region).getByRole('button', { name: Ar['targets.rollback.confirm'] }));

    expect(await within(region).findByText('البيئة متوقفة.')).toBeInTheDocument();
    expect(within(region).getByRole('button', { name: Ar['targets.rollback.confirm'] })).toBeInTheDocument();
  });

  it('offers no rollback while the environment is paused and says why', async () => {
    mount('owner', newTargetsState(), true);
    const region = await targetsRegion();
    await within(region).findByText('11-bbbbbbbb-1');
    expect(within(region).queryByRole('button', { name: Ar['targets.rollback.action'] })).not.toBeInTheDocument();
    expect(within(region).getByText(Ar['targets.paused'])).toBeInTheDocument();
  });
});

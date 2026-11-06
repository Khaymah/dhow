import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { findPage } from '@/Core/Testing/Pages';
import { HOST_ONE, HOST_THREE, HOST_TWO, newTargetsState, releaseFixture } from '@/Core/Testing/TargetsBackend';
import { API, ENVIRONMENT_URL, mountTargets, NEW_TARGET_URL, PASSWORD, TARGET_URL } from '@/Core/Testing/TargetsManageHarness';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('reaching the target pages from the environment', () => {
  it('links every target to its page and offers the form to an admin', async () => {
    const view = mountTargets({ url: ENVIRONMENT_URL, role: 'admin' });
    const main = await findPage('production');
    const region = await within(main).findByRole('region', { name: Ar['targets.group'] });
    const link = await within(region).findByRole('link', { name: Ar['targets.manage'] });
    expect(link).toHaveAttribute('href', `${ENVIRONMENT_URL}/targets/web`);

    await view.user.click(link);
    expect(await findPage('web')).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe(`${ENVIRONMENT_URL}/targets/web`);
  });

  it('opens the form from the add button', async () => {
    const view = mountTargets({ url: ENVIRONMENT_URL, role: 'owner' });
    const main = await findPage('production');
    await view.user.click(await within(main).findByRole('button', { name: Ar['targets.add'] }));
    expect(await findPage(Ar['targets.new.title'])).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe(NEW_TARGET_URL);
  });

  it('gives a maintainer the link but not the add button', async () => {
    mountTargets({ url: ENVIRONMENT_URL, role: 'maintainer' });
    const main = await findPage('production');
    expect(await within(main).findByRole('link', { name: Ar['targets.manage'] })).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['targets.add'] })).not.toBeInTheDocument();
  });
});

describe('the content security policy', () => {
  it.each([
    ['the target page', TARGET_URL],
    ['the form of a new target', NEW_TARGET_URL],
  ])('injects no style on %s', async (_, url) => {
    mountTargets({ url });
    await screen.findByRole('heading', { level: 1 });
    await screen.findByRole('main');
    await waitFor(() => {
      expect(document.querySelectorAll('input, textarea').length).toBeGreaterThan(0);
    });
    expect(document.querySelectorAll('style')).toHaveLength(0);
  });
});

describe('making a target', () => {
  it('sends the whole form with the second step and opens the new target', async () => {
    const view = mountTargets({ url: NEW_TARGET_URL });
    const main = await findPage(Ar['targets.new.title']);
    await view.user.type(await within(main).findByLabelText(Ar['targets.form.name']), 'api');
    await view.user.clear(within(main).getByLabelText(Ar['targets.form.username']));
    await view.user.type(within(main).getByLabelText(Ar['targets.form.username']), 'ship');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.path']), '/srv/api');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.host']), 'api-1.example.com');
    await view.user.click(within(main).getByRole('button', { name: Ar['targets.form.host.add'] }));
    const second = within(main).getAllByLabelText(Ar['targets.form.host'])[1];
    if (!second) throw new Error('the second server has no box');
    await view.user.type(second, '203.0.113.20');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.reload']), 'sudo systemctl reload api');
    await view.user.type(await within(main).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(main).getByRole('button', { name: Ar['targets.form.create'] }));

    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(1);
    });
    expect(view.manage.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/targets`,
      Body: {
        Name: 'api',
        Kind: 'ssh',
        Username: 'ship',
        DeployPath: '/srv/api',
        KeepReleases: 5,
        Settings: { Rolling: { BatchSize: 1, BatchPauseSeconds: 0 }, Ssh: { ReloadCommand: 'sudo systemctl reload api' } },
        AllowedImages: [],
        Hosts: [
          { Host: 'api-1.example.com', Port: 22 },
          { Host: '203.0.113.20', Port: 22 },
        ],
        CurrentFactor: { Method: 'password', Password: PASSWORD },
      },
    });
    expect(await screen.findByText(arText('targets.created', { name: 'api' }))).toBeInTheDocument();
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(`${ENVIRONMENT_URL}/targets/api`);
    });
  });

  it('sends only the compose block for a compose target', async () => {
    const view = mountTargets({ url: NEW_TARGET_URL });
    const main = await findPage(Ar['targets.new.title']);
    await view.user.type(await within(main).findByLabelText(Ar['targets.form.name']), 'stack');
    await view.user.selectOptions(within(main).getByLabelText(Ar['targets.form.kind']), 'compose');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.path']), '/srv/stack');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.host']), 'stack.example.com');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.projectName']), 'stack');
    expect(within(main).queryByLabelText(Ar['targets.form.reload'])).not.toBeInTheDocument();
    await view.user.type(await within(main).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(main).getByRole('button', { name: Ar['targets.form.create'] }));

    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(1);
    });
    expect(view.manage.writes[0]?.Body).toMatchObject({ Kind: 'compose', Settings: { Compose: { ProjectName: 'stack' } } });
  });

  it('refuses a form with a wrong name, path or user before it asks the server', async () => {
    const view = mountTargets({ url: NEW_TARGET_URL });
    const main = await findPage(Ar['targets.new.title']);
    await view.user.type(await within(main).findByLabelText(Ar['targets.form.name']), 'Bad Name');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.path']), '/srv/app');
    await view.user.type(within(main).getByLabelText(Ar['targets.form.host']), 'a.example.com');
    await view.user.type(await within(main).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(main).getByRole('button', { name: Ar['targets.form.create'] }));
    expect(await within(main).findByText(Ar['targets.form.error.name'])).toBeInTheDocument();

    await view.user.clear(within(main).getByLabelText(Ar['targets.form.name']));
    await view.user.type(within(main).getByLabelText(Ar['targets.form.name']), 'good');
    await view.user.clear(within(main).getByLabelText(Ar['targets.form.username']));
    await view.user.type(within(main).getByLabelText(Ar['targets.form.username']), 'root');
    await view.user.click(within(main).getByRole('button', { name: Ar['targets.form.create'] }));
    expect(await within(main).findByText(Ar['targets.form.error.root'])).toBeInTheDocument();
    expect(view.manage.writes).toHaveLength(0);
  });

  it('tells a maintainer that only admins add a target', async () => {
    mountTargets({ url: NEW_TARGET_URL, role: 'maintainer' });
    const main = await findPage(Ar['targets.new.title']);
    expect(await within(main).findByText(Ar['targets.new.forbidden'])).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['targets.form.create'] })).not.toBeInTheDocument();
  });
});

describe('changing and removing a target', () => {
  it('saves the settings with the version that was read', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const main = await findPage('web');
    const path = await within(main).findByDisplayValue('/srv/app');
    await view.user.clear(path);
    await view.user.type(path, '/srv/web');
    const form = path.closest('form');
    if (!form) throw new Error('the settings have no form');
    await view.user.type(within(form).getByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(form).getByRole('button', { name: Ar['targets.form.save'] }));

    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(1);
    });
    expect(view.manage.writes[0]).toMatchObject({
      Method: 'PUT',
      Path: `${API}/targets/web`,
      Body: { Version: 4, DeployPath: '/srv/web', Username: 'deploy', KeepReleases: 5, CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await screen.findByText(arText('targets.saved', { name: 'web' }))).toBeInTheDocument();
  });

  it('removes the target after the second step and returns to the environment', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const main = await findPage('web');
    await view.user.click(await within(main).findByRole('button', { name: Ar['targets.remove'] }));
    const panel = (await within(main).findByText(arText('targets.remove.question', { name: 'web' }))).parentElement;
    if (!panel) throw new Error('no panel');
    await view.user.type(within(panel).getByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(panel).getByRole('button', { name: Ar['targets.remove.confirm'] }));

    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(1);
    });
    expect(view.manage.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/targets/web/remove`,
      Body: { CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    await waitFor(() => {
      expect(view.router.state.location.pathname).toBe(ENVIRONMENT_URL);
    });
  });

  it('shows nothing to change to a developer of the project', async () => {
    mountTargets({ url: TARGET_URL, role: 'developer' });
    const main = await findPage('web');
    expect(await within(main).findByText(Ar['targets.manage.readOnly'])).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['targets.remove'] })).not.toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['targets.host.add'] })).not.toBeInTheDocument();
    expect(within(main).queryByLabelText(Ar['targets.keySecret.value'])).not.toBeInTheDocument();
  });
});

describe('the history of a target', () => {
  const history = () => {
    const targets = newTargetsState();
    targets.releases = {
      CurrentReleaseId: '12-aaaaaaaa-1',
      Partial: false,
      Releases: [
        releaseFixture('14-eeeeeeee-1', {
          State: 'rolled_back',
          RollbackTarget: false,
          RollbackReason: 'health_check_failed',
          Message: 'healthz answered 503',
          Hosts: [
            { HostId: HOST_ONE, Position: 1, State: 'rolled_back', RollbackReason: 'health_check_failed' },
            { HostId: HOST_TWO, Position: 2, State: 'failed', Message: 'healthz answered 503' },
            { HostId: HOST_THREE, Position: 3, State: 'skipped' },
          ],
        }),
        releaseFixture('13-cccccccc-1', { Kind: 'rollback', RestoredReleaseId: '12-aaaaaaaa-1', RollbackTarget: false }),
        releaseFixture('12-aaaaaaaa-1'),
        releaseFixture('10-dddddddd-1', { State: 'rollback_failed', RollbackTarget: false, RollbackReason: 'switch_failed' }),
      ],
    };
    return targets;
  };

  it('marks a deploy that rolled back by itself, with the reason and what happened on each server', async () => {
    mountTargets({ url: TARGET_URL, targets: history() });
    const main = await findPage('web');
    const row = (await within(main).findByText('14-eeeeeeee-1')).closest('li');
    if (!row) throw new Error('no row');

    expect(within(row).getByText(Ar['targets.release.auto'])).toBeInTheDocument();
    expect(within(row).getAllByText(Ar['targets.release.state.rolled_back']).length).toBeGreaterThan(0);
    expect(within(row).getByText(new RegExp(arText('targets.release.reason', { reason: Ar['targets.reason.health_check_failed'] })))).toBeInTheDocument();
    expect(within(row).getAllByText('healthz answered 503').length).toBeGreaterThan(0);
    expect(within(row).getByText('web-1.example.com:22')).toBeInTheDocument();
    expect(within(row).getByText(Ar['targets.release.host.skipped'])).toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: Ar['targets.rollback.action'] })).not.toBeInTheDocument();
  });

  it('tells kinds apart: a deploy, a manual rollback, a rollback that failed and the last check', async () => {
    const targets = history();
    targets.check = {
      Hosts: [
        { HostId: HOST_ONE, Position: 1, Host: 'web-1.example.com', Port: 22, Checked: true, Reason: 'ok', Locked: false, Repaired: false, CheckedAt: '2026-11-05T08:00:00Z' },
        { HostId: HOST_TWO, Position: 2, Host: 'web-2.example.com', Port: 22, Checked: true, Reason: 'disk_low', Locked: false, Repaired: false, CheckedAt: '2026-11-05T08:01:00Z' },
      ],
    };
    mountTargets({ url: TARGET_URL, targets });
    const main = await findPage('web');
    const manual = (await within(main).findByText('13-cccccccc-1')).closest('li');
    const failed = (await within(main).findByText('10-dddddddd-1')).closest('li');
    if (!manual || !failed) throw new Error('missing rows');

    expect(within(manual).getByText(new RegExp(arText('targets.release.kind.rollback', { release: '12-aaaaaaaa-1' })))).toBeInTheDocument();
    expect(within(manual).queryByText(Ar['targets.release.auto'])).not.toBeInTheDocument();
    expect(within(failed).getAllByText(Ar['targets.release.state.rollback_failed']).length).toBeGreaterThan(0);
    expect(await within(main).findByText(arText('targets.history.check.problems', { count: 1 }))).toBeInTheDocument();
    expect(within(main).getAllByText(Ar['targets.history.kind.check']).length).toBeGreaterThan(0);
  });

  it('shows the automatic rollback on the environment page too, without the per-server details', async () => {
    mountTargets({ url: ENVIRONMENT_URL, targets: history() });
    const main = await findPage('production');
    const region = await within(main).findByRole('region', { name: Ar['targets.group'] });
    expect(await within(region).findByText(Ar['targets.release.auto'])).toBeInTheDocument();
    expect(region.querySelector('[data-host-state]')).toBeNull();
  });
});

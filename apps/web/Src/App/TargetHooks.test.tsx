import { configure, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { findPage } from '@/Core/Testing/Pages';
import { newTargetsState, targetFixture } from '@/Core/Testing/TargetsBackend';
import { ENVIRONMENT_URL, mountTargets } from '@/Core/Testing/TargetsManageHarness';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

const withHooks = (ssh: Record<string, unknown>) => {
  const state = newTargetsState();
  state.targets = [targetFixture({ Settings: { Rolling: { BatchSize: 1, BatchPauseSeconds: 0 }, Ssh: ssh } })];
  return state;
};

const FLAGS_OFF = { PreSwitchCommandSet: false, PostSwitchCommandSet: false };

describe('the hooks of a target', () => {
  it('tells a viewer that hooks are set and hides their text', async () => {
    mountTargets({ url: ENVIRONMENT_URL, role: 'viewer', targets: withHooks({ ReloadCommandSet: true, ...FLAGS_OFF }) });
    const main = await findPage('production');
    expect(await within(main).findByText(Ar['targets.hooks.hidden'])).toBeInTheDocument();
  });

  it('says nothing when the text is shown to an admin', async () => {
    mountTargets({
      url: ENVIRONMENT_URL,
      role: 'admin',
      targets: withHooks({ ReloadCommand: 'sudo systemctl reload web', ReloadCommandSet: true, ...FLAGS_OFF }),
    });
    const main = await findPage('production');
    await within(main).findByRole('region', { name: Ar['targets.group'] });
    expect(within(main).queryByText(Ar['targets.hooks.hidden'])).not.toBeInTheDocument();
  });

  it('says nothing when the target has no hooks', async () => {
    mountTargets({ url: ENVIRONMENT_URL, role: 'viewer', targets: withHooks({ ReloadCommandSet: false, ...FLAGS_OFF }) });
    const main = await findPage('production');
    await within(main).findByRole('region', { name: Ar['targets.group'] });
    expect(within(main).queryByText(Ar['targets.hooks.hidden'])).not.toBeInTheDocument();
  });
});

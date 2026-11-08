import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { SETTINGS, settingVisible, type ISettingsContext } from '@/App/Navigation/SettingsRegistry';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { usePaletteStore } from '@/Core/Store/usePaletteStore';
import { startBackend } from '@/Core/Testing/Backend';
import { newNotificationsState, notificationsRoute } from '@/Core/Testing/NotificationsBackend';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { settingsFixture, settingsRoute } from '@/Core/Testing/SettingsBackend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const mount = (url: string, role: string, instanceOwner: boolean) => {
  startBackend({
    role,
    instanceOwner,
    routes: [
      notificationsRoute(newNotificationsState()),
      settingsRoute({ settings: settingsFixture(), puts: [], tests: 0 }),
      projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) }),
    ],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  return { router, user: userEvent.setup() };
};

const openPalette = async (user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement[]> => {
  await screen.findByRole('heading', { level: 1 });
  await user.keyboard('{Control>}k{/Control}');
  const list = await screen.findByRole('listbox', { name: Ar['palette.label'] });
  return within(list).findAllByRole('option');
};

const has = (options: HTMLElement[], id: string): boolean => options.some((option) => option.id.endsWith(`-setting:${id}`));

const PROJECT_SETTINGS_URL = `/projects/${PROJECT_ID}/settings`;

describe('the settings in the command palette', () => {
  afterEach(() => {
    usePaletteStore.getState().SetOpen(false);
  });

  it('lists every setting of the registry for the server owner who has a project open', async () => {
    const { user } = mount(PROJECT_SETTINGS_URL, 'owner', true);
    await waitFor(async () => {
      expect(await screen.findByText(projectFixture().Name as string)).toBeInTheDocument();
    });
    const options = await openPalette(user);
    await waitFor(() => {
      expect(has(options, 'project.danger')).toBe(true);
    });
    const context: ISettingsContext = { role: 'owner', owner: true, projectActive: true };
    for (const entry of SETTINGS) {
      expect(settingVisible(entry, context), entry.id).toBe(true);
      expect(has(options, entry.id), `${entry.id} has no palette entry`).toBe(true);
    }
  });

  it('leaves out the server settings and the danger zones for a viewer', async () => {
    const { user } = mount(PROJECT_SETTINGS_URL, 'viewer', false);
    const options = await openPalette(user);
    await waitFor(() => {
      expect(has(options, 'org.security')).toBe(true);
    });
    for (const id of ['system.mail', 'system.providers', 'system.logs', 'project.danger', 'project.deliveries']) {
      expect(has(options, id), id).toBe(false);
    }
    expect(has(options, 'security.sessions')).toBe(true);
    expect(has(options, 'preferences.notifications')).toBe(true);
  });

  it('leaves out the project settings when no project is open', async () => {
    const { user } = mount('/account/preferences', 'owner', true);
    const options = await openPalette(user);
    expect(has(options, 'org.general')).toBe(true);
    expect(has(options, 'system.mail')).toBe(true);
    expect(has(options, 'project.general')).toBe(false);
    expect(has(options, 'project.notifications')).toBe(false);
  });

  it('opens the page of a setting at its section', async () => {
    const { user, router } = mount('/account/preferences', 'owner', true);
    const options = await openPalette(user);
    const target = options.find((option) => option.id.endsWith('-setting:org.security'));
    if (target === undefined) throw new Error('no entry for the organization security');
    await user.click(target);
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/org/settings');
    });
    expect(router.state.location.hash.replace(/^#/, '')).toBe('security');
    const heading = await screen.findByRole('heading', { level: 2, name: Ar['orgSettings.group.security'] });
    await waitFor(() => {
      expect(heading.closest('[id="security"]')).toHaveFocus();
    });
  });
});

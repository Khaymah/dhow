import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { settingsOfPage, SETTINGS, settingVisible, type TSettingsPage } from '@/App/Navigation/SettingsRegistry';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { startBackend } from '@/Core/Testing/Backend';
import { newNotificationsState, notificationsRoute } from '@/Core/Testing/NotificationsBackend';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { settingsFixture, settingsRoute } from '@/Core/Testing/SettingsBackend';
import { SETTINGS_GROUPS } from '@/Modules/Settings/Types/SettingsGroups';

configure({ asyncUtilTimeout: 5000 });

const URLS: Record<TSettingsPage, string> = {
  org: '/org/settings',
  project: `/projects/${PROJECT_ID}/settings`,
  projectNotifications: `/projects/${PROJECT_ID}/notifications`,
  system: '/system/settings',
  security: '/account/security',
  preferences: '/account/preferences',
};

describe('the settings registry', () => {
  it('gives every setting one id and every anchor one place on its page', () => {
    expect(new Set(SETTINGS.map((entry) => entry.id)).size).toBe(SETTINGS.length);
    for (const page of Object.keys(URLS) as TSettingsPage[]) {
      const anchors = settingsOfPage(page).flatMap((entry) => (entry.anchor === null ? [] : [entry.anchor]));
      expect(new Set(anchors).size, page).toBe(anchors.length);
    }
  });

  it('lists the server settings cards of the page and its sign-in providers', () => {
    const anchors = settingsOfPage('system')
      .map((entry) => entry.anchor)
      .sort();
    expect(anchors).toEqual([...SETTINGS_GROUPS.map((spec) => spec.Path), 'providers'].sort());
  });

  it.each(Object.keys(URLS) as TSettingsPage[])('has the section of every %s setting on the page', async (page) => {
    startBackend({
      role: 'owner',
      instanceOwner: true,
      routes: [
        notificationsRoute(newNotificationsState()),
        settingsRoute({ settings: settingsFixture(), puts: [], tests: 0 }),
        projectsRoute({ project: () => ok(projectFixture({ CallerRole: 'owner' })) }),
      ],
    });
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: [URLS[page]] }))} queryClient={createQueryClient({ retry: false })} />);
    await screen.findByRole('heading', { level: 1 });
    for (const entry of settingsOfPage(page)) {
      if (entry.anchor === null || !settingVisible(entry, { role: 'owner', owner: true, projectActive: true })) continue;
      const anchor = entry.anchor;
      await waitFor(() => {
        expect(document.getElementById(anchor), `${entry.id} has no section #${anchor}`).not.toBeNull();
      });
    }
  });
});

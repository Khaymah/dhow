import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend } from '@/Core/Testing/Backend';
import { settingsFixture, settingsRoute } from '@/Core/Testing/SettingsBackend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const mount = (url: string, instanceOwner: boolean, role = 'owner') => {
  const backend = startBackend({ role, instanceOwner, routes: [settingsRoute({ settings: settingsFixture(), puts: [], tests: 0 })] });
  render(<App router={createAppRouter(createMemoryHistory({ initialEntries: [url] }))} queryClient={createQueryClient({ retry: false })} />);
  return { ...backend, user: userEvent.setup() };
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the server settings page', () => {
  it('sits under Monitoring for the server owner and opens from the menu', async () => {
    const { user } = mount('/system', true);
    const nav = await screen.findByRole('navigation', { name: Ar['shell.nav.label'] });
    const watch = within(nav).getByRole('group', { name: Ar['shell.group.watch'] });
    const links = within(watch)
      .getAllByRole('link')
      .map((link) => link.textContent);
    expect(links).toEqual([Ar['shell.nav.audit'], Ar['shell.nav.system'], Ar['shell.nav.systemSettings'], Ar['shell.nav.about']]);
    await user.click(within(watch).getByRole('link', { name: Ar['shell.nav.systemSettings'] }));
    expect(await screen.findByRole('heading', { level: 1, name: Ar['shell.nav.systemSettings'] })).toBeInTheDocument();
  });

  it.each(['owner', 'admin', 'viewer'])('is not found for an organization %s who does not own the server', async (role) => {
    const { calls } = mount('/system/settings', false, role);
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
    const nav = await screen.findByRole('navigation', { name: Ar['shell.nav.label'] });
    expect(within(nav).queryByRole('link', { name: Ar['shell.nav.systemSettings'] })).toBeNull();
    expect(calls.some((call) => new URL(call.url).pathname.includes('/instance/settings'))).toBe(false);
  });
});

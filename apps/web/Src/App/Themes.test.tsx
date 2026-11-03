import { createMemoryHistory } from '@tanstack/react-router';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { usePaletteStore } from '@/Core/Store/usePaletteStore';
import { useToastStore } from '@/Core/Store/useToastStore';
import { apiUp } from '@/Core/Testing/Fetch';
import { readStoredThemes } from '@/Core/Theme/ThemeStorage';
import { DEFAULT_COLORS } from '@/Core/Theme/ThemeTokens';
import { useThemeStore } from '@/Core/Theme/useThemeStore';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const html = document.documentElement;

const renderApp = async (url = '/') => {
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  const view = render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  await screen.findByRole('main', { hidden: true });
  return { router, user, view };
};

const runCommand = async (user: ReturnType<typeof userEvent.setup>, label: string) => {
  await user.keyboard('{Control>}k{/Control}');
  await user.type(await screen.findByRole('combobox'), label);
  await user.keyboard('{Enter}');
};

const findHex = (token: 'canvas' | 'rail') =>
  screen.findByLabelText<HTMLInputElement>(Ar['themeEditor.hexLabel'].replace('{name}', isolate(Ar[`theme.token.${token}`])));

beforeEach(() => {
  apiUp();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
  usePaletteStore.setState({ Open: false });
});

const preferences = () => screen.findByRole('heading', { level: 1, name: Ar['shell.nav.preferences'], hidden: true });

describe('opening appearance', () => {
  it('opens preferences from the account row and the account section links', async () => {
    const { router, user } = await renderApp();
    const nav = screen.getByRole('navigation', { name: Ar['shell.nav.label'] });
    await user.click(within(nav).getByRole('link', { name: /Sabry Dawood/ }));
    await screen.findByRole('heading', { level: 1, name: Ar['shell.nav.profile'] });
    const sections = screen.getByRole('navigation', { name: Ar['shell.sections.account'] });
    await user.click(within(sections).getByRole('link', { name: Ar['shell.nav.preferences'] }));
    await preferences();
    expect(router.state.location.href).toBe('/account/preferences');
    expect(within(sections).getByRole('link', { name: Ar['shell.nav.preferences'] })).toHaveAttribute('aria-current', 'page');
  });

  it('opens from the command palette', async () => {
    const { router, user } = await renderApp();
    await runCommand(user, Ar['palette.command.preferences']);
    await preferences();
    expect(router.state.location.href).toBe('/account/preferences');
  });

  it('opens the theme editor over preferences from the command palette', async () => {
    const { router, user } = await renderApp();
    await runCommand(user, Ar['palette.command.createTheme']);
    const dialog = await screen.findByRole('dialog', { name: Ar['themeEditor.defaultName'] });
    expect(router.state.location.href).toBe('/account/preferences?slide=theme-edit');
    await preferences();
    expect(await within(dialog).findByLabelText(Ar['themeEditor.name'])).toBeInTheDocument();
  });

  it('starts a theme from the page action of preferences', async () => {
    const { router, user } = await renderApp('/account/preferences');
    await user.click(await screen.findByRole('button', { name: Ar['appearance.create'] }));
    await screen.findByRole('dialog', { name: Ar['themeEditor.defaultName'] });
    expect(router.state.location.href).toBe('/account/preferences?slide=theme-edit');
  });

  it('reopens an existing custom theme from a deep link', async () => {
    useThemeStore.setState({ CustomThemes: [{ Id: 'mine', Name: 'Harbour', Colors: { ...DEFAULT_COLORS } }] });
    await renderApp('/account/preferences?slide=theme-edit-mine');
    const dialog = await screen.findByRole('dialog', { name: 'Harbour' });
    expect(await within(dialog).findByLabelText(Ar['themeEditor.name'])).toHaveValue('Harbour');
  });
});

describe('changing and editing themes in the app', () => {
  it('switches theme from appearance and keeps it after a reload', async () => {
    const first = await renderApp('/account/preferences');
    await first.user.click(await screen.findByRole('button', { name: Ar['theme.system.indigo'] }));
    expect(html).toHaveAttribute('data-theme', 'indigo');
    first.view.unmount();
    html.removeAttribute('data-theme');
    useThemeStore.setState({ ...readStoredThemes(), Draft: null });
    await renderApp('/account/preferences');
    expect(html).toHaveAttribute('data-theme', 'indigo');
    expect(await screen.findByRole('button', { name: Ar['theme.system.indigo'] })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('previews live, and Escape puts the active theme back', async () => {
    const { user } = await renderApp('/account/preferences?slide=theme-edit');
    fireEvent.change(await findHex('canvas'), { target: { value: '#123456' } });
    expect(html.style.getPropertyValue('--color-canvas')).toBe('#123456');
    await user.keyboard('{Escape}');
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(html.style.getPropertyValue('--color-canvas')).toBe('');
    });
    expect(html).toHaveAttribute('data-theme', 'sea');
    expect(useThemeStore.getState().CustomThemes).toEqual([]);
  });

  it('puts the active theme back when the slide is closed with its close button', async () => {
    const { user } = await renderApp('/account/preferences?slide=theme-edit');
    fireEvent.change(await findHex('rail'), { target: { value: '#123456' } });
    expect(html.style.getPropertyValue('--color-rail')).toBe('#123456');
    await user.click(screen.getByRole('button', { name: Ar['slides.closeAll'] }));
    await waitFor(() => {
      expect(html.style.getPropertyValue('--color-rail')).toBe('');
    });
  });

  it('follows the name in the title, then saves and goes back to preferences', async () => {
    const { router, user } = await renderApp('/account/preferences?slide=theme-edit');
    const name = await screen.findByLabelText(Ar['themeEditor.name']);
    await user.clear(name);
    await user.type(name, 'Harbour');
    expect(await screen.findByRole('dialog', { name: 'Harbour' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['themeEditor.save'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(router.state.location.href).toBe('/account/preferences');
    expect(await screen.findByText(Ar['appearance.inUse'])).toBeInTheDocument();
    expect(useThemeStore.getState().CustomThemes.map((theme) => theme.Name)).toEqual(['Harbour']);
  });
});

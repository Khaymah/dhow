import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { apiDown, apiUp, META_BODY } from '@/Core/Testing/Fetch';
import { useToastStore } from '@/Core/Store/useToastStore';
import { usePaletteStore } from '@/Core/Store/usePaletteStore';
import { En } from '@/I18n/Locales/En';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const renderApp = async (url = '/') => {
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  await screen.findByRole('main', { hidden: true });
  return { router, user };
};

beforeEach(() => {
  apiUp();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
  usePaletteStore.setState({ Open: false });
});

describe('app shell', () => {
  it('renders Arabic and right to left by default with one main area', async () => {
    await renderApp();
    expect(document.documentElement.dir).toBe('rtl');
    expect(document.documentElement.lang).toBe('ar');
    expect(screen.getAllByRole('main')).toHaveLength(1);
    expect(screen.getByRole('heading', { level: 1, name: Ar['home.title'] })).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: Ar['shell.nav.label'] });
    expect(within(nav).getByRole('link', { name: Ar['shell.nav.home'] })).toHaveAttribute('aria-current', 'page');
  });

  it('has a skip link that moves focus to the main area', async () => {
    const { user } = await renderApp();
    await user.click(screen.getByText(Ar['shell.skipToContent']));
    expect(screen.getByRole('main')).toHaveFocus();
  });

  it('opens the command palette from the search button and from Ctrl+K', async () => {
    const { user } = await renderApp();
    await user.click(screen.getByRole('button', { name: Ar['shell.search'] }));
    expect(await screen.findByRole('combobox', { name: Ar['palette.label'] })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => {
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    });
    await user.keyboard('{Control>}k{/Control}');
    expect(await screen.findByRole('combobox', { name: Ar['palette.label'] })).toHaveFocus();
  });

  it('runs a palette command and opens its page', async () => {
    const { router, user } = await renderApp();
    await user.keyboard('{Control>}k{/Control}');
    await user.type(await screen.findByRole('combobox'), Ar['palette.command.notifications']);
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('heading', { level: 1, name: Ar['notifications.title'] })).toBeInTheDocument();
    expect(router.state.location.href).toBe('/notifications');
  });

  it('runs a create command and opens its slide over the right page', async () => {
    const { router, user } = await renderApp();
    await user.keyboard('{Control>}k{/Control}');
    await user.type(await screen.findByRole('combobox'), Ar['palette.command.password']);
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('dialog', { name: Ar['password.slide.title'] })).toBeInTheDocument();
    expect(router.state.location.href).toBe('/account/security?slide=account-password');
    expect(screen.getByRole('heading', { level: 1, name: Ar['shell.nav.security'], hidden: true })).toBeInTheDocument();
  });
});

describe('slides in the URL', () => {
  it('opens a deep-linked slide over its page on first render and focuses inside it', async () => {
    await renderApp('/account/security?slide=account-password');
    const dialog = await screen.findByRole('dialog', { name: Ar['password.slide.title'] });
    expect(await within(dialog).findByLabelText(Ar['password.current'])).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: Ar['shell.nav.security'], hidden: true })).toBeInTheDocument();
    await waitFor(() => {
      expect(dialog).toContainElement(document.activeElement as HTMLElement);
    });
  });

  it('closes with Escape, drops the param, stays on the page and returns focus to the opener', async () => {
    const { router, user } = await renderApp('/account/security');
    const opener = await screen.findByRole('button', { name: Ar['account.password.change'] });
    await user.click(opener);
    await screen.findByRole('dialog', { name: Ar['password.slide.title'] });
    expect(router.state.location.href).toBe('/account/security?slide=account-password');
    await user.keyboard('{Escape}');
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(router.state.location.href).toBe('/account/security');
    await waitFor(() => {
      expect(opener).toHaveFocus();
    });
  });

  it('keeps an unknown slide closed, says so in a toast and cleans the URL', async () => {
    const { router } = await renderApp('/jobs?slide=does-not-exist');
    expect(await screen.findByText(Ar['toast.unknownPanel.title'])).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await waitFor(() => {
      expect(router.state.location.href).toBe('/jobs');
    });
  });
});

describe('language switch', () => {
  it('flips the whole shell to English and left to right, and remembers it', async () => {
    const { user } = await renderApp();
    await user.click(screen.getByRole('button', { name: Ar['shell.language.switch'].replace('{language}', isolate(Ar['language.en'])) }));
    expect(document.documentElement.dir).toBe('ltr');
    expect(document.documentElement.lang).toBe('en');
    expect(screen.getByRole('heading', { level: 1, name: En['home.title'] })).toBeInTheDocument();
    expect(within(screen.getByRole('navigation', { name: En['shell.nav.label'] })).getByRole('link', { name: En['shell.nav.home'] })).toBeInTheDocument();
    expect(window.localStorage.getItem('dhow.lang')).toBe('en');
    await user.click(screen.getByRole('button', { name: En['shell.language.switch'].replace('{language}', isolate(En['language.ar'])) }));
    expect(document.documentElement.dir).toBe('rtl');
  });
});

describe('unknown routes', () => {
  it('shows a not-found state that leads back to Home', async () => {
    const { router, user } = await renderApp('/nowhere');
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['notFound.action'] }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/');
    });
  });
});

describe('system module in the shell', () => {
  it('opens server status from the Home card and shows the build', async () => {
    const { router, user } = await renderApp();
    await user.click(await screen.findByRole('button', { name: Ar['home.status.open'] }));
    expect(await screen.findByRole('heading', { level: 1, name: Ar['status.title'] })).toBeInTheDocument();
    expect(await screen.findByText('v0.1.0')).toBeInTheDocument();
    expect(router.state.location.href).toBe('/system');
  });

  it('opens server status from a deep link', async () => {
    await renderApp('/system');
    expect(await screen.findByText('2e39188')).toBeInTheDocument();
  });

  it('shows the first-time setup banner when the server says setup is pending', async () => {
    apiUp({ ...META_BODY, Data: { ...META_BODY.Data, SetupRequired: true } });
    await renderApp();
    expect(await screen.findByText(Ar['setup.banner.title'])).toBeInTheDocument();
  });

  it('cannot open the shell while the API is down, and recovers when the person retries', async () => {
    apiDown();
    const router = createAppRouter(createMemoryHistory({ initialEntries: ['/'] }));
    const user = userEvent.setup();
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    expect(await screen.findByRole('heading', { level: 1, name: Ar['apiDown.title'] })).toBeInTheDocument();
    expect(screen.queryByRole('navigation', { name: Ar['shell.nav.label'] })).not.toBeInTheDocument();
    apiUp();
    await user.click(screen.getByRole('button', { name: Ar['apiDown.retry'] }));
    expect(await screen.findByRole('navigation', { name: Ar['shell.nav.label'] })).toBeInTheDocument();
  });
});

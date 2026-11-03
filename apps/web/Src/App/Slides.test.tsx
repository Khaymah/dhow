import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { usePaletteStore } from '@/Core/Store/usePaletteStore';
import { useToastStore } from '@/Core/Store/useToastStore';
import { apiUp, META_BODY } from '@/Core/Testing/Fetch';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const FAILING_READY = {
  Success: true,
  Data: {
    Status: 'unavailable',
    Checks: [
      { Name: 'database', Ok: false },
      { Name: 'migrations', Ok: true },
    ],
  },
};

const renderApp = async (url = '/') => {
  const history = createMemoryHistory({ initialEntries: [url] });
  const router = createAppRouter(history);
  const user = userEvent.setup();
  const view = render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  await screen.findByRole('main', { hidden: true });
  return { router, user, view, history };
};

const page = (title: string) => screen.findByRole('heading', { level: 1, name: title }, { timeout: 5000 });

const rowOf = (label: string): HTMLElement => {
  const row = within(screen.getByRole('main')).getByText(label).closest('button');
  if (!row) throw new Error(`${label} is not a row`);
  return row;
};

beforeEach(() => {
  apiUp();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
  usePaletteStore.setState({ Open: false });
});

describe('server status and its checks as pages', () => {
  it('opens a check from a deep link with the path in the top bar', async () => {
    await renderApp('/system/checks/database');
    await page(Ar['status.check.database']);
    const crumbs = screen.getByRole('navigation', { name: Ar['shell.crumbs.label'] });
    expect(within(crumbs).getAllByRole('listitem').map((item) => item.textContent)).toEqual([
      Ar['shell.group.watch'],
      Ar['shell.nav.system'],
      Ar['status.check.database'],
    ]);
    expect(within(crumbs).getByRole('link', { name: Ar['shell.nav.system'] })).toHaveAttribute('href', '/system');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('opens a check from its row as a page, and the back button returns to the list', async () => {
    const { router, user, history } = await renderApp('/system');
    await screen.findByText('v0.1.0');
    await user.click(rowOf(Ar['status.check.migrations']));
    await page(Ar['status.check.migrations']);
    expect(router.state.location.href).toBe('/system/checks/migrations');
    expect(await screen.findByText(Ar['checkDetail.migrations.means'])).toBeInTheDocument();
    expect(screen.getByText(Ar['checkDetail.okNow'])).toBeInTheDocument();
    history.back();
    await page(Ar['status.title']);
    expect(router.state.location.href).toBe('/system');
    history.forward();
    await page(Ar['status.check.migrations']);
  });

  it('tells what to do when a check fails', async () => {
    apiUp(META_BODY, FAILING_READY);
    await renderApp('/system/checks/database');
    expect(await screen.findByText(Ar['checkDetail.failNow'])).toBeInTheDocument();
    expect(screen.getByText(Ar['checkDetail.fail.title'])).toBeInTheDocument();
    expect(screen.getByText(Ar['checkDetail.database.fix2'])).toBeInTheDocument();
  });

  it('shows the not-found state for a check that does not exist', async () => {
    await renderApp('/system/checks/nothing');
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
  });

  it('shows the server state in the top bar and links it to the status page', async () => {
    apiUp(META_BODY, FAILING_READY);
    const { router, user } = await renderApp();
    const label = Ar['shell.status.open'].replace('{state}', isolate(Ar['shell.status.down']));
    const link = await screen.findByRole('link', { name: label });
    expect(link).toHaveAttribute('href', '/system');
    await user.click(link);
    await page(Ar['status.title']);
    expect(router.state.location.href).toBe('/system');
  });
});

describe('slides over a page', () => {
  it('opens a create slide over the current page, closes it and keeps the page', async () => {
    const { router, user } = await renderApp('/account/preferences');
    await page(Ar['shell.nav.preferences']);
    await user.click(screen.getByRole('button', { name: Ar['appearance.create'] }));
    const dialog = await screen.findByRole('dialog', { name: Ar['themeEditor.defaultName'] });
    expect(router.state.location.href).toBe('/account/preferences?slide=theme-edit');
    expect(within(dialog).queryByText('1 من 1')).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: Ar['slides.closeAll'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(router.state.location.href).toBe('/account/preferences');
  });

  it('round-trips: the URL of an open slide opens the same slide again', async () => {
    const first = await renderApp('/account/security');
    await first.user.click(await screen.findByRole('button', { name: Ar['account.password.change'] }));
    await screen.findByRole('dialog', { name: Ar['password.slide.title'] });
    const href = first.router.state.location.href;
    first.view.unmount();
    await renderApp(href);
    expect(await screen.findByRole('dialog', { name: Ar['password.slide.title'] })).toBeInTheDocument();
  });
});

describe('slides that are not valid', () => {
  it.each([
    '/jobs?slide=nope',
    '/jobs?slide=jobs',
    '/account/preferences?slide=theme-edit-ghost',
    '/account/preferences?slide=theme-edit-',
    '/?slide=%3Cscript%3E',
    '/?slide=5',
    '/?stack=system-status,nope',
    '/?stack=system-status,system-status',
    '/?stack=check-nothing',
    '/?stack=system-status,notifications,appearance,theme-edit,check-database,check-migrations,x',
    '/?stack=%3Cscript%3E',
    '/?stack=5',
  ])('drops %s, says so and keeps the page', async (url) => {
    const { router } = await renderApp(url);
    expect(await screen.findByText(Ar['toast.unknownPanel.title'])).toBeInTheDocument();
    await waitFor(() => {
      expect(router.state.location.searchStr).toBe('');
    });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});

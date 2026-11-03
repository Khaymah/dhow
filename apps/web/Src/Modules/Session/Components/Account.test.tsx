import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { backendFailure, startBackend, type IBackendOptions } from '@/Core/Testing/Backend';
import { useCsrfStore } from '@/Core/Store/useCsrfStore';
import { useToastStore } from '@/Core/Store/useToastStore';
import { findPage } from '@/Core/Testing/Pages';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const PROFILE = '/account/profile';

const mount = async (options: IBackendOptions = {}, url = '/account/security') => {
  const backend = startBackend(options);
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = await findPage(url === PROFILE ? Ar['shell.nav.profile'] : Ar['shell.nav.security']);
  return { ...backend, router, user, dialog };
};

const device = async (name: string) => within(await screen.findByRole('list', { name: Ar['account.sessions.title'] })).findByText(name);

const rowOf = async (name: string): Promise<HTMLElement> => {
  const row = (await device(name)).closest('li');
  if (!row) throw new Error(`no row for ${name}`);
  return row;
};

const methodCalls = (calls: Request[], method: string) => calls.filter((call) => call.method === method);

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('my account pages', () => {
  it('shows who is signed in and the role on the profile', async () => {
    const { dialog } = await mount({}, PROFILE);
    expect(await within(dialog).findAllByText('sabry@future.dev')).toHaveLength(1);
    expect(await within(dialog).findAllByText(Ar['role.owner'])).toHaveLength(1);
    expect(await within(dialog).findByText('Sabry Dawood')).toBeInTheDocument();
    expect(await within(dialog).findByText('Future Solution')).toBeInTheDocument();
  });

  it('says plainly that the account needs a second step while none is on, instead of a green all-clear', async () => {
    const { dialog } = await mount();
    const title = await within(dialog).findByText(Ar['account.summary.weak.title']);
    expect(title.closest('[data-tone]')).toHaveAttribute('data-tone', 'warn');
    expect(within(dialog).getByText(Ar['account.summary.weak.sub'])).toBeInTheDocument();
    expect(within(dialog).queryByText(Ar['account.summary.strong.title'])).not.toBeInTheDocument();
  });

  it('shows the green summary with the session end once a second step is on', async () => {
    const { dialog } = await mount({ mfa: { TotpEnabled: true, RecoveryCodesRemaining: 10, Passkeys: [] } });
    const title = await within(dialog).findByText(Ar['account.summary.strong.title']);
    expect(title.closest('[data-tone]')).toHaveAttribute('data-tone', 'ok');
    expect(within(dialog).queryByText(Ar['account.summary.weak.title'])).not.toBeInTheDocument();
  });

  it('opens the profile from the account link in the side menu', async () => {
    const backend = startBackend();
    const router = createAppRouter(createMemoryHistory({ initialEntries: ['/'] }));
    const user = userEvent.setup();
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    expect(backend.state.signedIn).toBe(true);
    const nav = await screen.findByRole('navigation', { name: Ar['shell.nav.label'] }, { timeout: 5000 });
    await user.click(within(nav).getByRole('link', { name: /Sabry Dawood/ }));
    expect(await findPage(Ar['shell.nav.profile'])).toBeInTheDocument();
    expect(router.state.location.href).toBe(PROFILE);
  });

  it('lists the devices with a readable name, address, last activity and which one is this device', async () => {
    const { dialog } = await mount();
    const chrome = await rowOf(`${isolate('Chrome')} على ${isolate('Linux')}`);
    expect(within(chrome).getByText(Ar['sessions.current'])).toBeInTheDocument();
    expect(within(chrome).queryByRole('button')).not.toBeInTheDocument();
    expect(within(chrome).getByText('203.0.113.7')).toBeInTheDocument();
    const firefox = await rowOf(`${isolate('Firefox')} على ${isolate('Windows')}`);
    expect(within(firefox).getByText(/منذ ساعتين/)).toBeInTheDocument();
    expect(within(firefox).getByRole('button', { name: Ar['sessions.revoke.aria'].replace('{device}', isolate(`${isolate('Firefox')} على ${isolate('Windows')}`)) })).toBeInTheDocument();
    expect(await rowOf(`${isolate('Safari')} على ${isolate('iOS')}`)).toBeInTheDocument();
    expect(within(dialog).getByText(arText('account.sessions.count', { count: 3 }))).toBeInTheDocument();
  });
});

describe('signing out one device', () => {
  it('asks first, sends nothing until confirmed, and can be cancelled', async () => {
    const { user, calls } = await mount();
    const firefox = await rowOf(`${isolate('Firefox')} على ${isolate('Windows')}`);
    await user.click(within(firefox).getByRole('button'));
    expect(within(firefox).getByText(Ar['sessions.revoke.question'])).toBeInTheDocument();
    expect(within(firefox).getByRole('button', { name: Ar['sessions.revoke.confirm'] })).toHaveFocus();
    expect(methodCalls(calls, 'DELETE')).toHaveLength(0);
    await user.click(within(firefox).getByRole('button', { name: Ar['sessions.revoke.cancel'] }));
    expect(within(firefox).queryByText(Ar['sessions.revoke.question'])).not.toBeInTheDocument();
    expect(methodCalls(calls, 'DELETE')).toHaveLength(0);
  });

  it('revokes the chosen session with the CSRF token and refreshes the list', async () => {
    const { user, calls, state } = await mount();
    const firefox = await rowOf(`${isolate('Firefox')} على ${isolate('Windows')}`);
    await user.click(within(firefox).getByRole('button'));
    await user.click(within(firefox).getByRole('button', { name: Ar['sessions.revoke.confirm'] }));
    await waitFor(() => {
      expect(screen.queryByText(`${isolate('Firefox')} على ${isolate('Windows')}`)).not.toBeInTheDocument();
    });
    const [revoke] = methodCalls(calls, 'DELETE');
    expect(new URL(revoke?.url ?? '').pathname).toBe('/api/v1/sessions/0198a000-0000-7000-8000-0000000000c2');
    expect(revoke?.headers.get('X-CSRF-Token')).toBe('csrf-token-1');
    expect(state.sessions).toHaveLength(2);
    expect(useToastStore.getState().Toasts.some((toast) => toast.Title === Ar['sessions.revoked'])).toBe(true);
  });

  it('shows the failure and keeps the device when the server refuses', async () => {
    const { user } = await mount({
      routes: [(request) => (request.method === 'DELETE' ? backendFailure('INTERNAL', 'حدث خطأ في السيرفر.', 500) : undefined)],
    });
    const firefox = await rowOf(`${isolate('Firefox')} على ${isolate('Windows')}`);
    await user.click(within(firefox).getByRole('button'));
    await user.click(within(firefox).getByRole('button', { name: Ar['sessions.revoke.confirm'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent('حدث خطأ في السيرفر.');
    expect(await rowOf(`${isolate('Firefox')} على ${isolate('Windows')}`)).toBeInTheDocument();
  });
});

describe('signing out every other device', () => {
  it('names how many devices go, then revokes them all and keeps this one', async () => {
    const { user, calls, state } = await mount();
    await user.click(await screen.findByRole('button', { name: Ar['sessions.revokeOthers'] }));
    expect(screen.getByText(arText('sessions.revokeOthers.question', { count: 2 }))).toBeInTheDocument();
    expect(screen.getByRole('button', { name: Ar['sessions.revokeOthers.confirm'] })).toHaveFocus();
    expect(methodCalls(calls, 'POST')).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: Ar['sessions.revokeOthers.confirm'] }));
    await waitFor(() => {
      expect(screen.queryByText(`${isolate('Firefox')} على ${isolate('Windows')}`)).not.toBeInTheDocument();
    });
    const [revoke] = methodCalls(calls, 'POST');
    expect(new URL(revoke?.url ?? '').pathname).toBe('/api/v1/sessions/revoke-others');
    expect(revoke?.headers.get('X-CSRF-Token')).toBe('csrf-token-1');
    expect(state.sessions).toHaveLength(1);
    expect(await rowOf(`${isolate('Chrome')} على ${isolate('Linux')}`)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['sessions.revokeOthers'] })).not.toBeInTheDocument();
    expect(useToastStore.getState().Toasts.some((toast) => toast.Title === arText('sessions.revokedOthers', { count: 2 }))).toBe(true);
  });

  it('offers nothing to revoke when this is the only device', async () => {
    const [current] = startBackend().state.sessions;
    await mount({ sessions: current ? [current] : [] });
    await rowOf(`${isolate('Chrome')} على ${isolate('Linux')}`);
    expect(screen.queryByRole('button', { name: Ar['sessions.revokeOthers'] })).not.toBeInTheDocument();
    expect(screen.getByText(Ar['account.sessions.count.one'])).toBeInTheDocument();
  });

  it('shows an error with a retry when the devices cannot be loaded', async () => {
    let failing = true;
    const { user } = await mount({
      routes: [
        (request, url) =>
          failing && request.method === 'GET' && url.pathname.endsWith('/sessions')
            ? backendFailure('INTERNAL', 'حدث خطأ في السيرفر.', 500)
            : undefined,
      ],
    });
    expect(await screen.findByText(Ar['account.sessions.error'])).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole('button', { name: Ar['status.retry'] }));
    expect(await rowOf(`${isolate('Chrome')} على ${isolate('Linux')}`)).toBeInTheDocument();
  });
});

describe('signing out', () => {
  it('ends the session, forgets the CSRF token and the cache, and lands on sign-in', async () => {
    const { user, router, calls, state, dialog } = await mount({}, PROFILE);
    await within(dialog).findByText('Sabry Dawood');
    await user.click(within(dialog).getByRole('button', { name: Ar['account.signOut'] }));
    expect(await screen.findByRole('heading', { level: 1, name: Ar['login.title'] })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe('/login');
    const logout = calls.find((call) => call.method === 'DELETE' && call.url.endsWith('/sessions/current'));
    expect(logout?.headers.get('X-CSRF-Token')).toBe('csrf-token-1');
    expect(state.signedIn).toBe(false);
    expect(useCsrfStore.getState().Token).toBeNull();
    expect(useToastStore.getState().Toasts.some((toast) => toast.Title === Ar['account.signOut.done'])).toBe(true);
  });

  it('signs out from the button next to the account link in the side menu', async () => {
    const { calls, state } = startBackend();
    const router = createAppRouter(createMemoryHistory({ initialEntries: ['/'] }));
    const user = userEvent.setup();
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    await user.click(await screen.findByRole('button', { name: Ar['account.signOut'] }));
    expect(await screen.findByRole('heading', { level: 1, name: Ar['login.title'] })).toBeInTheDocument();
    expect(state.signedIn).toBe(false);
    expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith('/sessions/current'))).toBe(true);
  });

  it('signs out from the command palette too', async () => {
    const { user } = await mount();
    await user.keyboard('{Control>}k{/Control}');
    await user.type(await screen.findByRole('combobox'), Ar['palette.command.signOut']);
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('heading', { level: 1, name: Ar['login.title'] })).toBeInTheDocument();
  });

  it('stays signed in and says so when the server cannot be reached', async () => {
    const { user, state, dialog } = await mount({
      routes: [
        (request) => {
          if (request.method === 'DELETE' && request.url.endsWith('/sessions/current')) throw new TypeError('Failed to fetch');
          return undefined;
        },
      ],
    }, PROFILE);
    await user.click(await within(dialog).findByRole('button', { name: Ar['account.signOut'] }));
    await waitFor(() => {
      expect(useToastStore.getState().Toasts.some((toast) => toast.Title === Ar['account.signOut.failed'])).toBe(true);
    });
    expect(state.signedIn).toBe(true);
    expect(screen.getByRole('heading', { level: 1, name: Ar['shell.nav.profile'] })).toBeInTheDocument();
  });
});

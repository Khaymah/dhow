import { screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, stubFetch } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { useCsrfStore } from '@/Core/Store/useCsrfStore';
import { Ar } from '@/I18n/Locales/Ar';
import { IdentitiesCard } from '@/Modules/Session/Components/IdentitiesCard';
import { SignInChoices } from '@/Modules/Session/Components/SignInChoices';
import { OidcCallbackPage } from '@/Modules/Session/Pages/OidcCallbackPage';
import { providerOf, rememberOidcPurpose, readOidcPurpose, takeOidcCallbackFromSearch } from '@/Modules/Session/Store/OidcCallback';
import { useOidcCallbackStore } from '@/Modules/Session/Store/useOidcCallbackStore';
import { isolate } from '@/I18n/Translate';

const PROVIDERS = [
  { Id: 'corp', Name: 'Corp SSO', Kind: 'oidc' },
  { Id: 'github', Name: 'GitHub', Kind: 'github' },
];
const AUTHORIZE = 'https://sso.example.com/authorize?client_id=dhow&state=s';
const LOGIN = { User: { Id: 'u', Email: 'sabry@future.dev', Name: 'Sabry', Locale: 'ar' }, CsrfToken: 'csrf-new', ExpiresAt: '2026-11-10T08:00:00Z' };
const path = (request: Request) => new URL(request.url).pathname;
const nth = <T,>(items: readonly T[], index: number): T => {
  const item = items[index];
  if (item === undefined) throw new Error(`missing item ${index}`);
  return item;
};

const label = (key: keyof typeof Ar, provider: string) => Ar[key].replace('{provider}', isolate(provider));

afterEach(() => {
  useOidcCallbackStore.getState().Take();
  useCsrfStore.getState().Clear();
  window.sessionStorage.clear();
});

describe('sign-in choices', () => {
  it('lists every configured provider and leaves for its authorization page', async () => {
    const { calls } = stubFetch((request) => {
      if (path(request).endsWith('/oidc/providers')) return envelope({ Success: true, Data: PROVIDERS });
      return envelope({ Success: true, Data: { AuthorizationUrl: AUTHORIZE, ExpiresAt: '2026-10-28T08:10:00Z' } });
    });
    const navigate = vi.fn();
    const { user } = renderWithProviders(<SignInChoices onSignedIn={vi.fn()} navigate={navigate} />);
    const corp = await screen.findByRole('button', { name: label('login.oidc.with', 'Corp SSO') });
    expect(screen.getByRole('button', { name: label('login.oidc.with', 'GitHub') })).toBeInTheDocument();
    await user.click(corp);
    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(AUTHORIZE);
    });
    const start = calls.find((call) => path(call).endsWith('/oidc/authorize'));
    expect(start?.method).toBe('POST');
    expect(await start?.clone().json()).toEqual({ Provider: 'corp' });
    expect(readOidcPurpose()).toBe('login');
  });

  it('shows the server error when a provider is down and lets the user try again', async () => {
    stubFetch((request) =>
      path(request).endsWith('/oidc/providers')
        ? envelope({ Success: true, Data: PROVIDERS })
        : backendFailure('OIDC_PROVIDER_UNAVAILABLE', 'خدمة الدخول غير متاحة.', 503),
    );
    const navigate = vi.fn();
    const { user } = renderWithProviders(<SignInChoices onSignedIn={vi.fn()} navigate={navigate} />);
    await user.click(await screen.findByRole('button', { name: label('login.oidc.with', 'Corp SSO') }));
    expect(await screen.findByText('خدمة الدخول غير متاحة.')).toBeInTheDocument();
    expect(navigate).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: label('login.oidc.with', 'Corp SSO') })).toBeEnabled();
  });
});

describe('the callback address', () => {
  it('takes the code and state out of the address and remembers them once', () => {
    expect(providerOf('/oidc/callback/corp')).toBe('corp');
    expect(providerOf('/oidc/callback/Evil')).toBeNull();
    expect(providerOf('/login')).toBeNull();
    expect(takeOidcCallbackFromSearch('/oidc/callback/corp', '?code=abc&state=xyz&iss=https%3A%2F%2Fsso')).toBe(true);
    expect(useOidcCallbackStore.getState().Take()).toEqual({ Provider: 'corp', Code: 'abc', State: 'xyz', Error: '' });
    expect(useOidcCallbackStore.getState().Take()).toBeNull();
    expect(takeOidcCallbackFromSearch('/oidc/callback/corp', '')).toBe(false);
    expect(takeOidcCallbackFromSearch('/oidc/callback/corp', `?code=${'a'.repeat(5000)}&state=s`)).toBe(true);
    expect(useOidcCallbackStore.getState().Take()?.Code).toBe('');
  });
});

const callbackPage = (handler: (request: Request) => Response) => {
  const { calls } = stubFetch(handler);
  const props = { onSignedIn: vi.fn(), onLinked: vi.fn(), onBack: vi.fn() };
  const view = renderWithProviders(<OidcCallbackPage provider="corp" {...props} />);
  return { ...view, ...props, calls };
};

describe('finishing the sign-in', () => {
  it('posts the code and state and signs in', async () => {
    useOidcCallbackStore.getState().Set({ Provider: 'corp', Code: 'abc', State: 'xyz', Error: '' });
    const { calls, onSignedIn } = callbackPage(() => envelope({ Success: true, Data: LOGIN }));
    await waitFor(() => {
      expect(onSignedIn).toHaveBeenCalledOnce();
    });
    expect(calls).toHaveLength(1);
    expect(path(nth(calls, 0))).toBe('/api/v1/oidc/callback');
    expect(await calls[0]?.clone().json()).toEqual({ Provider: 'corp', Code: 'abc', State: 'xyz' });
    expect(useCsrfStore.getState().Token).toBe('csrf-new');
  });

  it('continues to the second step when the account has one', async () => {
    useOidcCallbackStore.getState().Set({ Provider: 'corp', Code: 'abc', State: 'xyz', Error: '' });
    callbackPage(() => envelope({ Success: true, Data: { Methods: ['totp'], ExpiresAt: '2026-10-28T08:05:00Z' } }, 202));
    expect(await screen.findByRole('heading', { name: Ar['mfa.step.title'] })).toBeInTheDocument();
  });

  it('explains a cancelled or missing callback without calling the server', async () => {
    useOidcCallbackStore.getState().Set({ Provider: 'corp', Code: '', State: '', Error: 'access_denied' });
    const first = callbackPage(() => envelope({ Success: true, Data: LOGIN }));
    expect(await screen.findByText(Ar['oidc.callback.cancelled'])).toBeInTheDocument();
    expect(first.calls).toHaveLength(0);
    first.unmount();

    const second = callbackPage(() => envelope({ Success: true, Data: LOGIN }));
    expect(await screen.findByText(Ar['oidc.callback.missing'])).toBeInTheDocument();
    await second.user.click(screen.getByRole('button', { name: Ar['oidc.callback.backToLogin'] }));
    expect(second.onBack).toHaveBeenCalledWith('login');
    expect(second.calls).toHaveLength(0);
  });

  it('refuses a callback for another provider', async () => {
    useOidcCallbackStore.getState().Set({ Provider: 'github', Code: 'abc', State: 'xyz', Error: '' });
    const { calls } = callbackPage(() => envelope({ Success: true, Data: LOGIN }));
    expect(await screen.findByText(Ar['oidc.callback.missing'])).toBeInTheDocument();
    expect(calls).toHaveLength(0);
  });

  it('shows the server reason, such as an identity that is not linked', async () => {
    useOidcCallbackStore.getState().Set({ Provider: 'corp', Code: 'abc', State: 'xyz', Error: '' });
    callbackPage(() => backendFailure('OIDC_NOT_LINKED', 'لا يوجد حساب مربوط بهذه الهوية.', 401));
    expect(await screen.findByText('لا يوجد حساب مربوط بهذه الهوية.')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: Ar['oidc.callback.failed'] })).toBeInTheDocument();
  });

  it('finishes a link with the session CSRF token', async () => {
    rememberOidcPurpose('link');
    useOidcCallbackStore.getState().Set({ Provider: 'corp', Code: 'abc', State: 'xyz', Error: '' });
    const change = { Identity: { Id: 'i1', Provider: 'corp', ProviderName: 'Corp SSO', Email: 'sara@corp.example', CreatedAt: '2026-10-28T08:00:00Z' }, Revoked: 1 };
    const { calls, onLinked } = callbackPage((request) => (path(request).endsWith('/me') ? envelope(meBody()) : envelope({ Success: true, Data: change })));
    await waitFor(() => {
      expect(onLinked).toHaveBeenCalledWith(change);
    });
    const link = calls.find((call) => path(call).endsWith('/me/identities/callback'));
    expect(link?.headers.get('X-CSRF-Token')).toBe('csrf-token-1');
    expect(readOidcPurpose()).toBe('login');
  });
});

const mfaOff = { Enabled: false, TotpEnabled: false, RecoveryCodesRemaining: 0, PasskeyCount: 0 };

describe('linked sign-in accounts', () => {
  const identity = { Id: 'i1', Provider: 'corp', ProviderName: 'Corp SSO', Email: 'sara@corp.example', CreatedAt: '2026-10-01T08:00:00Z' };
  const backend = (identities: unknown[], extra: (request: Request) => Response | undefined = () => undefined) =>
    stubFetch((request) => {
      const handled = extra(request);
      if (handled) return handled;
      const p = path(request);
      if (p.endsWith('/oidc/providers')) return envelope({ Success: true, Data: PROVIDERS });
      if (p.endsWith('/me/identities')) return envelope({ Success: true, Data: identities });
      if (p.endsWith('/me/mfa')) return envelope({ Success: true, Data: mfaOff });
      return backendFailure('NOT_FOUND', 'x', 404);
    });

  it('shows which providers are linked', async () => {
    backend([identity]);
    renderWithProviders(<IdentitiesCard />);
    const list = await screen.findByRole('list', { name: Ar['identities.title'] });
    const rows = within(list).getAllByRole('listitem');
    expect(rows).toHaveLength(2);
    expect(within(nth(rows, 0)).getByText(/sara@corp\.example/)).toBeInTheDocument();
    expect(within(nth(rows, 0)).getByRole('button', { name: Ar['identities.unlink'] })).toBeInTheDocument();
    expect(within(nth(rows, 1)).getByRole('button', { name: Ar['identities.link'] })).toBeInTheDocument();
  });

  it('links after confirming the current factor', async () => {
    const { calls } = backend([], (request) =>
      path(request).endsWith('/me/identities/authorize') ? envelope({ Success: true, Data: { AuthorizationUrl: AUTHORIZE, ExpiresAt: 'x' } }) : undefined,
    );
    const navigate = vi.fn();
    const { user } = renderWithProviders(<IdentitiesCard navigate={navigate} />);
    const rows = await screen.findAllByRole('listitem');
    await user.click(within(nth(rows, 1)).getByRole('button', { name: Ar['identities.link'] }));
    await user.type(screen.getByLabelText(Ar['factor.method.password']), 'correct horse battery staple');
    await user.click(screen.getByRole('button', { name: label('identities.link.confirm', 'GitHub') }));
    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(AUTHORIZE);
    });
    const start = calls.find((call) => path(call).endsWith('/me/identities/authorize'));
    expect(await start?.clone().json()).toEqual({ Provider: 'github', CurrentFactor: { Method: 'password', Password: 'correct horse battery staple' } });
    expect(readOidcPurpose()).toBe('link');
  });

  it('unlinks after confirming the factor and refuses the last way in with the server message', async () => {
    backend([identity], (request) =>
      path(request).endsWith('/remove') ? backendFailure('LAST_SIGN_IN_METHOD', 'هذه آخر طريقة للدخول.', 409) : undefined,
    );
    const { user } = renderWithProviders(<IdentitiesCard />);
    await user.click(await screen.findByRole('button', { name: Ar['identities.unlink'] }));
    await user.type(screen.getByLabelText(Ar['factor.method.password']), 'correct horse battery staple');
    await user.click(screen.getByRole('button', { name: Ar['identities.unlink.confirm'] }));
    expect(await screen.findByText('هذه آخر طريقة للدخول.')).toBeInTheDocument();
  });

  it('stays out of the way when nothing is configured or linked', async () => {
    stubFetch((request) => envelope({ Success: true, Data: path(request).endsWith('/me/mfa') ? mfaOff : [] }));
    const { container } = renderWithProviders(<IdentitiesCard />);
    await waitFor(() => {
      expect(container).toBeEmptyDOMElement();
    });
  });
});

import { screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';
import { GOOD_CODE } from '@/Core/Testing/MfaBackend';
import {
  oidcFixture,
  oidcProviderFixture,
  oidcRoute,
  type IOidcBackend,
  type IOidcReplies,
  type TOidcFixture,
} from '@/Core/Testing/OidcSettingsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { OidcCard } from '@/Modules/Settings/Components/OidcCard';
import { isolate } from '@/I18n/Translate';

const TOTP = { TotpEnabled: true, RecoveryCodesRemaining: 10, Passkeys: [] };

const mount = (oidc: TOidcFixture, replies: IOidcReplies = {}) => {
  const state: IOidcBackend = { oidc, writes: [], tests: [] };
  const onAdd = vi.fn();
  const onEdit = vi.fn();
  const backend = startBackend({ instanceOwner: true, mfa: TOTP, routes: [oidcRoute(state, replies)] });
  const view = renderWithProviders(<OidcCard onAdd={onAdd} onEdit={onEdit} />);
  return { ...backend, ...view, state, onAdd, onEdit };
};

const fill = (key: keyof typeof Ar, values: Record<string, string>): string =>
  Object.entries(values).reduce<string>((text, [name, value]) => text.replace(`{${name}}`, isolate(value)), Ar[key]);

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the sign-in providers card', () => {
  it('shows a loading state while the list loads', async () => {
    mount(oidcFixture(), { get: () => new Promise<Response>(() => undefined) as unknown as Response });
    expect(await screen.findByRole('status', { name: Ar['settings.oidc.loading'] })).toBeInTheDocument();
  });

  it('shows an error with a retry that loads the list', async () => {
    let failed = false;
    const { user } = mount(oidcFixture(), {
      get: () => {
        if (failed) return envelope({ Success: true, Data: oidcFixture() });
        failed = true;
        return backendFailure('INTERNAL', 'x', 500);
      },
    });
    expect(await screen.findByText(Ar['settings.oidc.error'])).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['settings.retry'] }));
    expect(await screen.findByRole('button', { name: Ar['settings.oidc.add'] })).toBeInTheDocument();
  });

  it('invites the owner to add the first provider', async () => {
    const { user, onAdd } = mount(oidcFixture());
    expect(await screen.findByText(fill('settings.oidc.empty', { google: 'Google', github: 'GitHub', oidc: 'OpenID Connect' }))).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['settings.oidc.add'] }));
    expect(onAdd).toHaveBeenCalledOnce();
  });

  it('lists each provider with its type, issuer and linked accounts', async () => {
    const off = oidcProviderFixture('staff', {
      Name: 'Staff',
      Enabled: false,
      ClientSecret: { Set: false, Saved: false, Locked: false, Env: 'X' },
    });
    const { user, onEdit } = mount(oidcFixture([oidcProviderFixture('corp', { LinkedIdentities: 3 }), off]));
    const list = await screen.findByRole('list', { name: Ar['settings.oidc.list'] });
    const rows = within(list).getAllByRole('listitem');
    expect(rows).toHaveLength(2);
    const [corp, staff] = rows;
    if (corp === undefined || staff === undefined) throw new Error('rows missing');
    expect(within(corp).getByText('Corp SSO')).toBeInTheDocument();
    expect(within(corp).getByText('https://sso.corp.example')).toBeInTheDocument();
    expect(within(corp).getByText(fill('settings.oidc.linked', { count: '3' }))).toBeInTheDocument();
    expect(within(staff).getByText(Ar['settings.oidc.off'])).toBeInTheDocument();
    await user.click(within(corp).getByRole('button', { name: Ar['settings.oidc.edit'] }));
    expect(onEdit).toHaveBeenCalledWith('corp');
  });

  it('shows providers from the environment read-only', async () => {
    const locked = { ...oidcFixture([oidcProviderFixture('staff')], true), IgnoredStored: 2 };
    mount(locked);
    expect(await screen.findByText(Ar['settings.oidc.locked'])).toBeInTheDocument();
    expect(screen.getByText('DHOW_OIDC_PROVIDERS')).toBeInTheDocument();
    expect(screen.getByText(fill('settings.oidc.ignored', { count: '2' }))).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['settings.oidc.add'] })).toBeNull();
    expect(screen.queryByRole('button', { name: Ar['settings.oidc.edit'] })).toBeNull();
    expect(screen.queryByRole('button', { name: Ar['settings.oidc.remove'] })).toBeNull();
    expect(screen.getByRole('button', { name: Ar['settings.oidc.test'] })).toBeEnabled();
    expect(screen.queryByText(Ar['settings.oidc.stranded'])).toBeNull();
  });

  it('warns when the environment list leaves linked accounts without their provider', async () => {
    mount({ ...oidcFixture([oidcProviderFixture('staff')], true), IgnoredStored: 1, IgnoredLinkedIdentities: 3 });
    const note = (await screen.findByText(Ar['settings.oidc.stranded'])).closest('[role="note"]');
    expect(note).not.toBeNull();
    expect(note).toHaveTextContent(fill('settings.oidc.strandedAbout', { count: '3' }));
  });

  it('never warns about stranded accounts while the dashboard list is in effect', async () => {
    mount({ ...oidcFixture([oidcProviderFixture()]), IgnoredLinkedIdentities: 3 });
    expect(await screen.findByText(Ar['settings.oidc.title'])).toBeInTheDocument();
    expect(screen.queryByText(Ar['settings.oidc.stranded'])).toBeNull();
  });

});

describe('testing and removing a provider', () => {
  it('tests a saved provider and shows each check', async () => {
    const { user, state } = mount(oidcFixture([oidcProviderFixture()]), {
      test: () =>
        envelope({
          Success: true,
          Data: {
            Passed: false,
            Checks: [
              { Check: 'discovery', Passed: true },
              { Check: 'endpoints', Passed: false },
              { Check: 'keys', Passed: false },
            ],
          },
        }),
    });
    await user.click(await screen.findByRole('button', { name: Ar['settings.oidc.test'] }));
    expect(await screen.findByText(Ar['settings.oidc.test.failed'])).toBeInTheDocument();
    expect(screen.getByText(Ar['settings.oidc.check.discovery'])).toBeInTheDocument();
    expect(screen.getAllByText(Ar['settings.oidc.check.failed'])).toHaveLength(2);
    expect(state.tests).toEqual([{ Type: 'oidc', Issuer: 'https://sso.corp.example' }]);
  });

  it('removes a provider after naming the linked accounts and a fresh factor', async () => {
    const { user, state } = mount(oidcFixture([oidcProviderFixture('corp', { LinkedIdentities: 2, Version: 4 })]));
    await user.click(await screen.findByRole('button', { name: Ar['settings.oidc.remove'] }));
    expect(await screen.findByText(`${fill('settings.oidc.remove.question', { name: 'Corp SSO', count: '2' })} ${Ar['settings.oidc.remove.about']}`)).toBeInTheDocument();
    await user.type(await screen.findByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(screen.getByRole('button', { name: fill('settings.oidc.remove.confirm', { name: 'Corp SSO' }) }));
    await waitFor(() => {
      expect(useToastStore.getState().Toasts.some((toast) => toast.Title === Ar['settings.oidc.removed'])).toBe(true);
    });
    expect(state.writes).toEqual([
      { method: 'POST', path: '/api/v1/instance/settings/oidc/corp/remove', body: { Version: 4, CurrentFactor: { Method: 'totp', Code: GOOD_CODE } } },
    ]);
    expect(await screen.findByRole('button', { name: Ar['settings.oidc.add'] })).toBeInTheDocument();
  });

  it('refuses to remove a provider that some accounts need to sign in', async () => {
    const { user, state } = mount(oidcFixture([oidcProviderFixture('corp', { LinkedIdentities: 2, OnlySignInUsers: 1 })]));
    await user.click(await screen.findByRole('button', { name: Ar['settings.oidc.remove'] }));
    expect(await screen.findByText(Ar['settings.oidc.remove.blocked'])).toBeInTheDocument();
    expect(screen.getByText(fill('settings.oidc.remove.blockedAbout', { count: '1' }))).toBeInTheDocument();
    expect(screen.queryByLabelText(Ar['factor.method.totp'])).toBeNull();
    expect(state.writes).toEqual([]);
  });

  it('shows the server refusal when an account became stranded meanwhile', async () => {
    const { user } = mount(oidcFixture([oidcProviderFixture('corp', { LinkedIdentities: 1 })]), {
      write: () =>
        envelope(
          { Success: false, Error: { Code: 'OIDC_PROVIDER_ONLY_SIGN_IN', Message: 'بعض الحسابات لا تدخل إلا بهذا المزوّد.', Details: [{ Field: 'Users', Message: '1' }] } },
          409,
        ),
    });
    await user.click(await screen.findByRole('button', { name: Ar['settings.oidc.remove'] }));
    await user.type(await screen.findByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(screen.getByRole('button', { name: fill('settings.oidc.remove.confirm', { name: 'Corp SSO' }) }));
    expect(await screen.findByText('بعض الحسابات لا تدخل إلا بهذا المزوّد.')).toBeInTheDocument();
  });
});

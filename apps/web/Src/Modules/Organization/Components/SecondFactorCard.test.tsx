import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { formatDateTime } from '@/Core/Format/TimeFormat';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend, type IBackendOptions, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID } from '@/Core/Testing/Fetch';
import { GOOD_CODE } from '@/Core/Testing/MfaBackend';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const RULE_PATH = `/api/v1/organizations/${ORG_ID}/second-factor-rule`;
const SINCE = '2026-10-20T08:00:00Z';

const ruleBackend = (initial: 'off' | 'admins' | 'everyone') => {
  const state = { Rule: initial };
  const organization = () => ({
    Id: ORG_ID,
    Name: 'Future Solution',
    Slug: 'future-solution',
    CreatedAt: '2026-10-01T08:00:00Z',
    RequireSecondFactor: state.Rule,
    ...(state.Rule === 'off' ? {} : { RequireSecondFactorSince: SINCE }),
  });
  const route: TRouteHandler = (request, url) => {
    if (url.pathname === `/api/v1/organizations/${ORG_ID}` && request.method === 'GET') return envelope({ Success: true, Data: organization() });
    return undefined;
  };
  return { state, organization, routes: [route] };
};

const mountSettings = async (options: IBackendOptions) => {
  const backend = startBackend(options);
  const router = createAppRouter(createMemoryHistory({ initialEntries: ['/org/settings'] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = await findPage(Ar['shell.nav.orgSettings']);
  return { ...backend, user, dialog };
};

const meWithDue = (due: string): TRouteHandler => (request, url) => {
  if (!url.pathname.endsWith('/me') || request.method !== 'GET') return undefined;
  const me = meBody('admin');
  return envelope({ ...me, Data: { ...me.Data, Memberships: me.Data.Memberships.map((m) => ({ ...m, SecondFactorDue: due })) } });
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the second-step rule of the organization', () => {
  it('asks an owner without a second step to turn on their own first', async () => {
    const rules = ruleBackend('off');
    const { dialog } = await mountSettings({ routes: rules.routes });
    expect(await within(dialog).findByText(Ar['organization.secondFactor.ownFirst'])).toBeInTheDocument();
    expect(within(dialog).queryByRole('radio', { name: new RegExp(Ar['organization.secondFactor.rule.admins']) })).toBeNull();
  });

  it('lets an owner with TOTP require it from admins after confirming a code', async () => {
    const rules = ruleBackend('off');
    const saveRule: TRouteHandler = (request, url) => {
      if (url.pathname !== RULE_PATH || request.method !== 'PUT') return undefined;
      rules.state.Rule = 'admins';
      return envelope({ Success: true, Message: 'identity.secondFactor.saved', Data: rules.organization() });
    };
    const { user, dialog, calls } = await mountSettings({
      routes: [saveRule, ...rules.routes],
      mfa: { TotpEnabled: true, RecoveryCodesRemaining: 10, Passkeys: [] },
    });
    await user.click(await within(dialog).findByRole('radio', { name: new RegExp(Ar['organization.secondFactor.rule.admins']) }));
    expect(within(dialog).getByText(Ar['organization.secondFactor.question.wider'])).toBeInTheDocument();
    await user.type(await within(dialog).findByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(within(dialog).getByRole('button', { name: Ar['organization.secondFactor.confirm.admins'] }));
    await waitFor(() => {
      expect(useToastStore.getState().Toasts.some((toast) => toast.Title === Ar['organization.secondFactor.saved.admins'])).toBe(true);
    });
    const put = calls.find((call) => call.method === 'PUT' && new URL(call.url).pathname === RULE_PATH);
    expect(put?.headers.get('X-CSRF-Token')).toBe('csrf-token-1');
    expect(await put?.clone().json()).toEqual({ RequireSecondFactor: 'admins', CurrentFactor: { Method: 'totp', Code: GOOD_CODE } });
    expect(await within(dialog).findByText(Ar['organization.secondFactor.since'].replace('{date}', isolate(formatDateTime(SINCE, 'ar'))))).toBeInTheDocument();
  });

  it('warns before lowering the protection and shows the server reason on failure', async () => {
    const rules = ruleBackend('everyone');
    const refuse: TRouteHandler = (request, url) => {
      if (url.pathname !== RULE_PATH || request.method !== 'PUT') return undefined;
      return envelope({ Success: false, Error: { Code: 'INVALID_CURRENT_FACTOR', Message: 'لم نتمكن من التأكد من هويتك.' } }, 403);
    };
    const { user, dialog } = await mountSettings({
      routes: [refuse, ...rules.routes],
      mfa: { TotpEnabled: true, RecoveryCodesRemaining: 10, Passkeys: [] },
    });
    await user.click(await within(dialog).findByRole('radio', { name: new RegExp(Ar['organization.secondFactor.rule.off']) }));
    expect(within(dialog).getByText(Ar['organization.secondFactor.question.narrower'])).toBeInTheDocument();
    await user.type(await within(dialog).findByLabelText(Ar['factor.method.totp']), '000000');
    await user.click(within(dialog).getByRole('button', { name: Ar['organization.secondFactor.confirm.off'] }));
    expect(await within(dialog).findByText('لم نتمكن من التأكد من هويتك.')).toBeInTheDocument();
  });

  it('shows an admin the rule without a way to change it', async () => {
    const { dialog } = await mountSettings({ role: 'admin', secondFactorRule: 'admins' });
    await within(dialog).findByLabelText(Ar['organization.name']);
    expect(within(dialog).getByText(Ar['organization.secondFactor.rule.admins'])).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['orgSettings.security.ownerOnly'])).toBeInTheDocument();
    expect(within(dialog).queryByRole('radiogroup', { name: Ar['organization.secondFactor.title'] })).toBeNull();
  });
});

describe('the second-step deadline banner', () => {
  const mountHome = (routes: TRouteHandler[]) => {
    const backend = startBackend({ role: 'admin', routes });
    const router = createAppRouter(createMemoryHistory({ initialEntries: ['/'] }));
    const user = userEvent.setup();
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    return { ...backend, user };
  };
  const title = Ar['secondFactor.banner.title'].replace('{organization}', isolate('Future Solution'));

  it('shows the deadline during the grace period and opens the account', async () => {
    const due = new Date(Date.now() + 3 * 86_400_000).toISOString();
    const { user } = mountHome([meWithDue(due)]);
    const banner = (await screen.findByText(title)).closest('[role="status"]');
    expect(banner).not.toBeNull();
    expect(within(banner as HTMLElement).getByText(Ar['secondFactor.banner.before'].replace('{date}', isolate(formatDateTime(due, 'ar'))))).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['secondFactor.banner.action'] }));
    expect(await findPage(Ar['shell.nav.security'])).toBeInTheDocument();
  });

  it('turns into an alert after the deadline', async () => {
    const due = new Date(Date.now() - 3_600_000).toISOString();
    mountHome([meWithDue(due)]);
    const banner = (await screen.findByText(title)).closest('[role="alert"]');
    expect(banner).not.toBeNull();
    expect(within(banner as HTMLElement).getByText(Ar['secondFactor.banner.after'])).toBeInTheDocument();
  });
});

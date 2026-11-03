import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch, USER_ID } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { TokenNewBody } from '@/Modules/Tokens/Components/TokenNewBody';
import { TokensCard } from '@/Modules/Tokens/Components/TokensCard';
import { isolate } from '@/I18n/Translate';

const VALUE = 'dhow_pat_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcd';
const path = (request: Request) => new URL(request.url).pathname;
const nth = <T,>(items: readonly T[], index: number): T => {
  const item = items[index];
  if (item === undefined) throw new Error(`missing item ${index}`);
  return item;
};

const mfaOff = { Enabled: false, TotpEnabled: false, RecoveryCodesRemaining: 0, PasskeyCount: 0 };

const token = (over: Record<string, unknown> = {}) => ({
  Id: '0198a000-0000-7000-8000-0000000000d1',
  OrganizationId: ORG_ID,
  OrganizationName: 'Future Solution',
  Name: 'Deploy bot',
  Scopes: ['org.read', 'member.read'],
  Status: 'active',
  Owner: { Id: USER_ID, Name: 'Sabry Dawood', Email: 'sabry@future.dev' },
  CreatedAt: '2026-10-20T08:00:00Z',
  ExpiresAt: '2026-11-20T08:00:00Z',
  LastUsedIp: '',
  ...over,
});

describe('the API tokens card', () => {
  it('lists the tokens with their state and never shows a value', async () => {
    stubFetch(() =>
      envelope({
        Success: true,
        Data: [token(), token({ Id: '0198a000-0000-7000-8000-0000000000d2', Name: 'Old CI', Status: 'revoked', Scopes: ['org.read'] })],
      }),
    );
    renderWithProviders(<TokensCard onNew={vi.fn()} />);
    const list = await screen.findByRole('list', { name: Ar['tokens.title'] });
    const items = within(list).getAllByRole('listitem');
    const [active, revoked] = [nth(items, 0), nth(items, 1)];
    expect(within(active).getByText(Ar['tokens.status.active'])).toBeInTheDocument();
    expect(within(active).getByText(/Future Solution/)).toBeInTheDocument();
    expect(within(revoked).getByText(Ar['tokens.status.revoked'])).toBeInTheDocument();
    expect(within(revoked).queryByRole('button')).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain('dhow_pat_');
  });

  it('revokes a token after asking', async () => {
    const { calls } = stubFetch((request) =>
      request.method === 'POST' ? envelope({ Success: true, Data: token({ Status: 'revoked' }) }) : envelope({ Success: true, Data: [token()] }),
    );
    const { user } = renderWithProviders(<TokensCard onNew={vi.fn()} />);
    await user.click(await screen.findByRole('button', { name: Ar['tokens.revoke.aria'].replace('{name}', isolate('Deploy bot')) }));
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: Ar['tokens.revoke.confirm'] }));
    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST' && path(call).endsWith(`/me/api-tokens/${token().Id}/revoke`))).toBe(true);
    });
  });

  it('opens the new-token slide', async () => {
    stubFetch(() => envelope({ Success: true, Data: [] }));
    const onNew = vi.fn();
    const { user } = renderWithProviders(<TokensCard onNew={onNew} />);
    expect(await screen.findByText(Ar['tokens.empty'])).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['tokens.new'] }));
    expect(onNew).toHaveBeenCalledOnce();
  });
});

const newToken = (role: string, create: (request: Request) => Response) => {
  const { calls } = stubFetch((request) => {
    const p = path(request);
    if (p.endsWith('/me')) return envelope(meBody(role));
    if (p.endsWith('/me/mfa')) return envelope({ Success: true, Data: mfaOff });
    if (p.endsWith('/api-tokens') && request.method === 'POST') return create(request);
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onDone = vi.fn();
  return { ...renderWithProviders(<TokenNewBody onDone={onDone} />), calls, onDone };
};

describe('creating an API token', () => {
  it('offers only the permissions the role allows', async () => {
    newToken('developer', () => envelope({ Success: true, Data: {} }));
    const scopes = await screen.findByRole('group', { name: Ar['tokens.form.scopes'] });
    expect(within(scopes).getByRole('checkbox', { name: new RegExp(Ar['scope.member.manage']) })).toBeDisabled();
    expect(within(scopes).getByRole('checkbox', { name: new RegExp(Ar['scope.member.read']) })).toBeEnabled();
    expect(within(scopes).getByRole('checkbox', { name: new RegExp(Ar['scope.org.read']) })).toHaveAttribute('aria-checked', 'true');
  });

  it('sends the name, scopes, expiry and factor, then shows the value once', async () => {
    const { user, calls, onDone } = newToken('owner', () => envelope({ Success: true, Data: { Token: token({ Name: 'CI' }), Value: VALUE } }));
    await user.type(await screen.findByLabelText(Ar['tokens.form.name']), '  CI  ');
    const scopes = screen.getByRole('group', { name: Ar['tokens.form.scopes'] });
    await user.click(within(scopes).getByRole('checkbox', { name: new RegExp(Ar['scope.runner.manage']) }));
    await user.click(screen.getByRole('radio', { name: arText('tokens.form.days', { count: 90 }) }));
    await user.type(screen.getByLabelText(Ar['factor.method.password']), 'correct horse battery staple');
    await user.click(screen.getByRole('button', { name: Ar['tokens.form.submit'] }));
    expect(await screen.findByText(VALUE)).toBeInTheDocument();
    const sent = nth(calls.filter((call) => call.method === 'POST'), 0);
    expect(path(sent)).toBe(`/api/v1/organizations/${ORG_ID}/api-tokens`);
    expect(await sent.clone().json()).toEqual({
      Name: 'CI', Scopes: ['org.read', 'runner.manage'], ExpiresInDays: 90,
      CurrentFactor: { Method: 'password', Password: 'correct horse battery staple' },
    });
    await user.click(screen.getByRole('button', { name: Ar['tokens.created.done'] }));
    expect(onDone).toHaveBeenCalledOnce();
  });

  it('checks the form before asking the server', async () => {
    const { user, calls } = newToken('owner', () => envelope({ Success: true, Data: {} }));
    await user.click(await screen.findByRole('button', { name: Ar['tokens.form.submit'] }));
    expect(await screen.findByText(Ar['tokens.form.error.name'])).toBeInTheDocument();
    await user.type(screen.getByLabelText(Ar['tokens.form.name']), 'CI');
    const scopes = screen.getByRole('group', { name: Ar['tokens.form.scopes'] });
    await user.click(within(scopes).getByRole('checkbox', { name: new RegExp(Ar['scope.org.read']) }));
    await user.click(screen.getByRole('button', { name: Ar['tokens.form.submit'] }));
    expect(await screen.findByText(Ar['tokens.form.error.scopes'])).toBeInTheDocument();
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
  });

  it('shows the server refusal, such as a wrong factor, and keeps the form', async () => {
    const { user } = newToken('owner', () => backendFailure('INVALID_CURRENT_FACTOR', 'لم نتمكن من التأكد من هويتك.', 403));
    await user.type(await screen.findByLabelText(Ar['tokens.form.name']), 'CI');
    await user.type(screen.getByLabelText(Ar['factor.method.password']), 'wrong password here');
    await user.click(screen.getByRole('button', { name: Ar['tokens.form.submit'] }));
    expect(await screen.findByText('لم نتمكن من التأكد من هويتك.')).toBeInTheDocument();
    expect(screen.getByLabelText(Ar['tokens.form.name'])).toHaveValue('CI');
  });
});

import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch, USER_ID } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import {
  OrganizationTokenNewBody,
  OrganizationTokensBody,
  OrganizationTokensFooter,
} from '@/Modules/Tokens/Components/OrganizationTokensBody';
import { isolate } from '@/I18n/Translate';

const VALUE = 'dhow_pat_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcd';
const TOKEN_ID = '0198a000-0000-7000-8000-0000000000d1';
const path = (request: Request) => new URL(request.url).pathname;

const token = (over: Record<string, unknown> = {}) => ({
  Id: TOKEN_ID,
  OrganizationId: ORG_ID,
  OrganizationName: 'Future Solution',
  Name: 'Deploy bot',
  Scopes: ['org.read', 'runner.manage'],
  Status: 'active',
  Owner: { Id: USER_ID, Name: 'Mona Adel', Email: 'mona@future.dev' },
  CreatedAt: '2026-10-20T08:00:00Z',
  ExpiresAt: '2026-11-20T08:00:00Z',
  LastUsedIp: '',
  ...over,
});

const backend = (role: string, tokens: (request: Request) => Response | Promise<Response>) =>
  stubFetch((request) => {
    const p = path(request);
    if (p.endsWith('/me')) return envelope(meBody(role));
    if (p.endsWith('/me/mfa')) return envelope({ Success: true, Data: { Enabled: false, TotpEnabled: false, RecoveryCodesRemaining: 0, PasskeyCount: 0 } });
    if (p.includes('/api-tokens')) return tokens(request);
    return backendFailure('NOT_FOUND', 'x', 404);
  });

describe('the organization API tokens slide', () => {
  it('lists every token with its owner, scopes, dates and state, and never a value', async () => {
    backend('admin', () =>
      envelope({
        Success: true,
        Data: [
          token({ LastUsedAt: '2026-10-21T08:00:00Z' }),
          token({ Id: '0198a000-0000-7000-8000-0000000000d2', Name: 'Old CI', Status: 'revoked', RevokedAt: '2026-10-22T08:00:00Z' }),
        ],
      }),
    );
    renderWithProviders(<OrganizationTokensBody onNew={vi.fn()} />);
    const list = await screen.findByRole('list', { name: Ar['orgTokens.list'] });
    const [active, revoked] = within(list).getAllByRole('listitem');
    if (active === undefined || revoked === undefined) throw new Error('missing rows');
    expect(active).toHaveTextContent('Deploy bot');
    expect(active).toHaveTextContent(`${Ar['orgTokens.createdBy']} Mona Adel`);
    expect(within(active).getByText(Ar['scope.runner.manage'])).toBeInTheDocument();
    expect(within(active).getByText(Ar['orgTokens.lastUsed'])).toBeInTheDocument();
    expect(within(active).getByText(Ar['orgTokens.expires'])).toBeInTheDocument();
    expect(within(revoked).getByText(Ar['orgTokens.revokedAt'])).toBeInTheDocument();
    expect(within(revoked).queryByRole('button')).not.toBeInTheDocument();
    expect(screen.getByText(arText('orgTokens.count', { count: 1 }))).toBeInTheDocument();
    expect(document.body.textContent).not.toContain('dhow_pat_');
  });

  it('revokes any token of the organization only after asking', async () => {
    const { calls } = backend('owner', (request) =>
      request.method === 'POST' ? envelope({ Success: true, Data: token({ Status: 'revoked' }) }) : envelope({ Success: true, Data: [token()] }),
    );
    const { user } = renderWithProviders(<OrganizationTokensBody onNew={vi.fn()} />);
    await user.click(await screen.findByRole('button', { name: Ar['tokens.revoke.aria'].replace('{name}', isolate('Deploy bot')) }));
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: Ar['tokens.revoke.confirm'] }));
    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST' && path(call) === `/api/v1/organizations/${ORG_ID}/api-tokens/${TOKEN_ID}/revoke`)).toBe(true);
    });
  });

  it('shows a loading state, then an empty list that offers a new token', async () => {
    let release: (response: Response) => void = () => undefined;
    backend('admin', () => new Promise<Response>((resolve) => (release = resolve)));
    const onNew = vi.fn();
    const { user } = renderWithProviders(<OrganizationTokensBody onNew={onNew} />);
    expect(await screen.findByLabelText(Ar['orgTokens.loading'])).toBeInTheDocument();
    release(envelope({ Success: true, Data: [] }));
    expect(await screen.findByText(Ar['orgTokens.empty'])).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['tokens.new'] }));
    expect(onNew).toHaveBeenCalledOnce();
  });

  it('shows an error with a retry', async () => {
    let failing = true;
    backend('admin', () => (failing ? backendFailure('INTERNAL', 'حدث خطأ في السيرفر.', 500) : envelope({ Success: true, Data: [token()] })));
    const { user } = renderWithProviders(<OrganizationTokensBody onNew={vi.fn()} />);
    expect(await screen.findByText(Ar['orgTokens.error'])).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole('button', { name: Ar['status.retry'] }));
    expect(await screen.findByText('Deploy bot')).toBeInTheDocument();
  });

  it('is closed to roles that cannot manage tokens, without asking the API', async () => {
    const { calls } = backend('developer', () => envelope({ Success: true, Data: [token()] }));
    renderWithProviders(
      <>
        <OrganizationTokensBody onNew={vi.fn()} />
        <OrganizationTokensFooter onNew={vi.fn()} />
      </>,
    );
    expect(await screen.findByText(Ar['orgTokens.forbidden'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['tokens.new'] })).not.toBeInTheDocument();
    expect(calls.some((call) => path(call).includes('/api-tokens'))).toBe(false);
  });

  it('creates a token for the current organization and shows the value once', async () => {
    const { calls } = backend('owner', () => envelope({ Success: true, Data: { Token: token({ Name: 'CI' }), Value: VALUE } }));
    const onDone = vi.fn();
    const { user } = renderWithProviders(<OrganizationTokenNewBody onDone={onDone} />);
    await user.type(await screen.findByLabelText(Ar['tokens.form.name']), 'CI');
    expect(screen.queryByRole('radiogroup', { name: Ar['tokens.form.organization'] })).not.toBeInTheDocument();
    await user.type(screen.getByLabelText(Ar['factor.method.password']), 'correct horse battery staple');
    await user.click(screen.getByRole('button', { name: Ar['tokens.form.submit'] }));
    expect(await screen.findByText(VALUE)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: Ar['tokens.created.copy'] })).toBeInTheDocument();
    const sent = calls.find((call) => call.method === 'POST');
    expect(sent && path(sent)).toBe(`/api/v1/organizations/${ORG_ID}/api-tokens`);
    await user.click(screen.getByRole('button', { name: Ar['tokens.created.done'] }));
    expect(onDone).toHaveBeenCalledOnce();
  });
});

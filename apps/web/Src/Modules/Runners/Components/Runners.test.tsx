import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch, USER_ID } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { RegistrationTokensCard } from '@/Modules/Runners/Components/RegistrationTokensCard';
import { RunnersBody, RunnersFooter } from '@/Modules/Runners/Components/RunnersBody';
import { isolate } from '@/I18n/Translate';

const RUNNER_ID = '0198a000-0000-7000-8000-0000000000e1';
const TOKEN_ID = '0198a000-0000-7000-8000-0000000000f1';
const path = (request: Request) => new URL(request.url).pathname;

const runner = (over: Record<string, unknown> = {}) => ({
  Id: RUNNER_ID,
  OrganizationId: ORG_ID,
  Name: 'build-01',
  Labels: ['linux-builders'],
  CapabilityLabels: ['dhow.os=linux', 'dhow.arch=amd64', 'dhow.isolation=container'],
  Os: 'linux',
  Arch: 'amd64',
  Version: 'v0.3.0',
  IsolationLevels: ['container'],
  Capacity: 2,
  State: 'active',
  Status: 'online',
  KeyVersion: 1,
  KeysRotatedAt: '2026-10-28T08:00:00Z',
  LastSeenAt: new Date().toISOString(),
  CreatedAt: '2026-10-28T08:00:00Z',
  ...over,
});

const token = (over: Record<string, unknown> = {}) => ({
  Id: TOKEN_ID,
  Description: 'Build box',
  Labels: ['linux-builders'],
  MaxUses: 2,
  Uses: 1,
  ExpiresAt: '2026-10-28T23:00:00Z',
  Status: 'active',
  CreatedAt: '2026-10-28T21:00:00Z',
  CreatedByUserId: USER_ID,
  ...over,
});

const page = (items: unknown[]) => envelope({ Success: true, Data: items, Meta: { Page: 1, Limit: 20, Total: items.length } });

interface IBackend {
  role?: string;
  runners?: () => Response | Promise<Response>;
  tokens?: () => Response;
  revokeToken?: () => Response;
}

const mount = (
  { role = 'owner', runners = () => page([runner()]), tokens = () => page([token()]), revokeToken }: IBackend = {},
  withTokens = false,
  selectedId?: string,
) => {
  const { calls } = stubFetch((request) => {
    const p = path(request);
    if (p.endsWith('/me')) return envelope(meBody(role));
    if (p.endsWith('/runners')) return runners();
    if (p.endsWith('/revoke') && revokeToken) return revokeToken();
    if (p.endsWith('/runner-registration-tokens')) return tokens();
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onOpenRunner = vi.fn();
  const onAddRunner = vi.fn();
  const view = renderWithProviders(
    <>
      <RunnersBody onOpenRunner={onOpenRunner} onAddRunner={onAddRunner} hrefFor={(id) => `/runners/${id}`} selectedId={selectedId} />
      {withTokens ? <RegistrationTokensCard organizationId={ORG_ID} /> : null}
      <footer>
        <RunnersFooter onAddRunner={onAddRunner} />
      </footer>
    </>,
  );
  return { ...view, calls, onOpenRunner, onAddRunner };
};

describe('the runners list', () => {
  it('shows a skeleton while the runners load', async () => {
    mount({ runners: () => new Promise<Response>(() => undefined) });
    expect(await screen.findByRole('status', { name: Ar['runners.loading'] })).toBeInTheDocument();
  });

  it('lists the runners with their state and opens one', async () => {
    const { user, onOpenRunner } = mount({
      runners: () => page([runner(), runner({ Id: '0198a000-0000-7000-8000-0000000000e2', Name: 'build-02', State: 'disabled', Status: 'disabled', DisabledReason: 'manual' })]),
    });
    const list = await screen.findByRole('list', { name: Ar['runners.list.title'] });
    expect(within(list).getByText(Ar['runners.view.online'])).toBeInTheDocument();
    expect(within(list).getByText(Ar['runners.view.paused'])).toBeInTheDocument();
    const link = within(list).getByRole('link', { name: /build-01/ });
    expect(link).toHaveAttribute('href', `/runners/${RUNNER_ID}`);
    expect(link).not.toHaveAttribute('aria-current');
    await user.click(link);
    expect(onOpenRunner).toHaveBeenCalledWith(RUNNER_ID);
  });

  it('marks the selected runner', async () => {
    mount({ runners: () => page([runner(), runner({ Id: '0198a000-0000-7000-8000-0000000000e2', Name: 'build-02' })]) }, false, RUNNER_ID);
    const list = await screen.findByRole('list', { name: Ar['runners.list.title'] });
    expect(within(list).getByRole('link', { name: /build-01/ })).toHaveAttribute('aria-current', 'page');
    expect(within(list).getByRole('link', { name: /build-02/ })).not.toHaveAttribute('aria-current');
  });

  it('explains how to add the first runner, with the register command', async () => {
    const { user, onAddRunner } = mount({ runners: () => page([]) });
    expect(await screen.findByText(Ar['runners.empty.manager'])).toBeInTheDocument();
    expect(screen.getByLabelText(Ar['runners.command.label'])).toHaveTextContent(
      `sudo -u dhow-runner dhow-runner register --dev --url ${window.location.origin} --token-file -`,
    );
    const adds = screen.getAllByRole('button', { name: Ar['runners.add'] });
    expect(adds).toHaveLength(2);
    await user.click(adds[0] ?? document.body);
    expect(onAddRunner).toHaveBeenCalledOnce();
  });

  it('shows the error and retries', async () => {
    let fail = true;
    const { user } = mount({ runners: () => (fail ? backendFailure('INTERNAL', 'وقع خطأ في السيرفر.', 500) : page([runner()])) });
    expect(await screen.findByText('وقع خطأ في السيرفر.')).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole('button', { name: Ar['runners.retry'] }));
    expect(await screen.findByRole('list', { name: Ar['runners.list.title'] })).toBeInTheDocument();
  });

  it('offers a developer the list only: no tokens and no add button', async () => {
    const { calls } = mount({ role: 'developer', runners: () => page([]) });
    expect(await screen.findByText(Ar['runners.empty.member'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['runners.add'] })).not.toBeInTheDocument();
    expect(screen.queryByText(Ar['runners.tokens.title'])).not.toBeInTheDocument();
    expect(calls.some((call) => path(call).endsWith('/runner-registration-tokens'))).toBe(false);
  });

  it('leaves the registration tokens to their own page, even for an owner', async () => {
    const { calls } = mount();
    expect(await screen.findByRole('list', { name: Ar['runners.list.title'] })).toBeInTheDocument();
    expect(screen.queryByText(Ar['runners.tokens.title'])).not.toBeInTheDocument();
    expect(calls.some((call) => path(call).endsWith('/runner-registration-tokens'))).toBe(false);
  });

  it('keeps viewers out without asking the server', async () => {
    const { calls } = mount({ role: 'viewer' });
    expect(await screen.findByText(Ar['runners.forbidden'])).toBeInTheDocument();
    expect(calls.some((call) => path(call).includes('/runners'))).toBe(false);
  });
});

describe('the registration tokens card', () => {
  it('lists the tokens without any secret', async () => {
    mount({ tokens: () => page([token(), token({ Id: '0198a000-0000-7000-8000-0000000000f2', Description: '', Status: 'used_up', Uses: 2 })]) }, true);
    const list = await screen.findByRole('list', { name: Ar['runners.tokens.title'] });
    const [active, used] = within(list).getAllByRole('listitem');
    expect(within(active ?? list).getByText(Ar['runners.token.status.active'])).toBeInTheDocument();
    expect(within(used ?? list).getByText(Ar['runners.token.noDescription'])).toBeInTheDocument();
    expect(within(used ?? list).queryByRole('button')).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain('dhow_rrt_');
  });

  it('shows an empty list and a loading error', async () => {
    mount({ tokens: () => page([]) }, true);
    expect(await screen.findByText(Ar['runners.tokens.empty'])).toBeInTheDocument();
  });

  it('shows the error when the tokens cannot load', async () => {
    mount({ tokens: () => backendFailure('INTERNAL', 'وقع خطأ في السيرفر.', 500) }, true);
    expect(await screen.findByText(Ar['runners.tokens.error'])).toBeInTheDocument();
  });

  it('revokes a token only after asking', async () => {
    const { user, calls } = mount({ revokeToken: () => envelope({ Success: true, Data: token({ Status: 'revoked' }) }) }, true);
    await user.click(await screen.findByRole('button', { name: Ar['runners.token.revoke.aria'].replace('{name}', isolate('Build box')) }));
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: Ar['runners.token.revoke.confirm'] }));
    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST' && path(call).endsWith(`/runner-registration-tokens/${TOKEN_ID}/revoke`))).toBe(true);
    });
  });
});

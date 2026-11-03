import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend } from '@/Core/Testing/Backend';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

configure({ asyncUtilTimeout: 5000 });

const open = async (role = 'owner', url = '/org/settings') => {
  const backend = startBackend({
    role,
    routes: [
      (request, url) =>
        request.method === 'POST' && url.pathname.endsWith('/leave') ? backendFailure('LAST_OWNER', 'x', 409) : undefined,
    ],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const page = await findPage(Ar['shell.nav.orgSettings']);
  return { ...backend, router, user, page };
};

const dangerOf = (page: HTMLElement) => within(page).findByRole('region', { name: Ar['orgSettings.group.danger'] });

const leaveName = Ar['organization.leave.action'].replace('{name}', isolate('Future Solution'));

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the organization settings page', () => {
  it('groups the settings into general, security and a danger zone', async () => {
    const { page } = await open();
    for (const name of [Ar['orgSettings.group.general'], Ar['orgSettings.group.security'], Ar['orgSettings.group.danger']]) {
      expect(await within(page).findByRole('heading', { level: 2, name })).toBeInTheDocument();
    }
    const danger = await dangerOf(page);
    expect(danger.querySelector('[data-variant="primary"]')).toBeNull();
    expect(within(danger).getAllByRole('button').every((button) => button.dataset.variant === 'danger')).toBe(true);
  });

  it.each([
    ['owner', { rename: true, rule: true, transfer: true }],
    ['admin', { rename: true, rule: false, transfer: false }],
    ['maintainer', { rename: false, rule: false, transfer: false }],
    ['developer', { rename: false, rule: false, transfer: false }],
    ['viewer', { rename: false, rule: false, transfer: false }],
  ] as const)('shows a %s only what the role may change', async (role, may) => {
    const { page } = await open(role);
    const danger = await dangerOf(page);
    expect(within(danger).getByRole('button', { name: leaveName })).toBeInTheDocument();
    expect(within(danger).queryByRole('button', { name: Ar['organization.transfer.open'] }) !== null).toBe(may.transfer);
    expect(within(page).queryByLabelText(Ar['organization.name']) !== null).toBe(may.rename);
    if (may.rule) {
      expect(await within(page).findByText(Ar['organization.secondFactor.ownFirst'])).toBeInTheDocument();
      expect(within(page).queryByText(Ar['orgSettings.security.ownerOnly'])).not.toBeInTheDocument();
    } else {
      expect(within(page).getByText(Ar['orgSettings.security.ownerOnly'])).toBeInTheDocument();
    }
  });
});

describe('the danger zone', () => {
  it('opens the ownership transfer in a slide', async () => {
    const { page, user, router } = await open();
    await user.click(within(await dangerOf(page)).getByRole('button', { name: Ar['organization.transfer.open'] }));
    expect(await screen.findByRole('dialog', { name: Ar['organization.transfer.title'] })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ slide: 'organization-transfer' });
  });

  it('asks before leaving, sends nothing on cancel, and explains the last-owner refusal', async () => {
    const { page, user, calls } = await open();
    const danger = await dangerOf(page);
    await user.click(within(danger).getByRole('button', { name: leaveName }));
    const slide = await screen.findByRole('dialog', { name: Ar['organization.leave.title'] });
    expect(await within(slide).findByText(Ar['organization.leave.ownerNote'])).toBeInTheDocument();
    await user.click(await within(slide).findByRole('button', { name: Ar['organization.leave.cancel'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(calls.some((call) => call.method === 'POST')).toBe(false);

    await user.click(within(danger).getByRole('button', { name: leaveName }));
    const again = await screen.findByRole('dialog', { name: Ar['organization.leave.title'] });
    await user.click(await within(again).findByRole('button', { name: Ar['organization.leave.confirm'] }));
    expect(await within(again).findByText(Ar['organization.leave.lastOwner'])).toBeInTheDocument();
  });
});

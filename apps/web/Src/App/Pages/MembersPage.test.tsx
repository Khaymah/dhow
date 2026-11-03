import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, membersFixture, startBackend, type IBackendOptions } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { orgBackend, TEAM_ID } from '@/Core/Testing/OrgBackend';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

configure({ asyncUtilTimeout: 5000 });

const HANA = '0198a000-0000-7000-8000-0000000000b3';
const MONA = '0198a000-0000-7000-8000-0000000000b2';
const LIST_PAGE = `/api/v1/organizations/${ORG_ID}/members?Page=1&Limit=25`;

const open = async (url: string, options: IBackendOptions = {}) => {
  const org = orgBackend();
  const backend = startBackend({ ...options, routes: [...(options.routes ?? []), ...org.routes] });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const page = await findPage(Ar['shell.nav.members']);
  return { ...backend, router, user, page };
};

const listOf = (page: HTMLElement) => within(page).findByRole('list', { name: Ar['members.title'] });

const linkTo = (list: HTMLElement, name: string) => {
  const link = within(list).getByText(name).closest('a');
  if (link === null) throw new Error(`no link for ${name}`);
  return link;
};

const listRequests = (calls: readonly Request[]) => calls.filter((call) => call.method === 'GET' && call.url.endsWith(LIST_PAGE)).length;

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the members page', () => {
  it('shows the list alone until a member is picked', async () => {
    const { page } = await open('/org/members');
    const list = await listOf(page);
    expect(within(list).getAllByRole('link')).toHaveLength(3);
    expect(page.querySelector('[data-master-detail]')).toBeNull();
    expect(within(list).queryByRole('link', { current: 'page' })).toBeNull();
  });

  it('puts the member beside the list on wide screens and alone with a way back below them', async () => {
    const { page } = await open(`/org/members/${HANA}`);
    const list = await listOf(page);
    expect(await within(page).findByRole('heading', { level: 2, name: 'Hana Samir' })).toBeInTheDocument();
    expect(page.querySelector('[data-master-list]')).toHaveClass('max-xl:hidden');
    expect(page.querySelector('[data-master-list]')?.parentElement).toHaveClass('xl:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]');
    expect(within(list).getByRole('link', { current: 'page' })).toHaveTextContent('Hana Samir');
    const back = within(page).getByRole('link', { name: Ar['members.back'] });
    expect(back).toHaveAttribute('href', '/org/members');
    expect(back).toHaveClass('xl:hidden');
  });

  it('switches members without remounting or refetching the list', async () => {
    const { page, user, calls, router } = await open(`/org/members/${HANA}`);
    const list = await listOf(page);
    await within(page).findByRole('heading', { level: 2, name: 'Hana Samir' });
    expect(listRequests(calls)).toBe(1);

    await user.click(linkTo(list, 'Mona Adel'));
    expect(await within(page).findByRole('heading', { level: 2, name: 'Mona Adel' })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/org/members/${MONA}`);
    expect(await listOf(page)).toBe(list);
    expect(within(list).getByRole('link', { current: 'page' })).toHaveTextContent('Mona Adel');
    await user.click(linkTo(list, 'Hana Samir'));
    await within(page).findByRole('heading', { level: 2, name: 'Hana Samir' });
    expect(await listOf(page)).toBe(list);
    expect(listRequests(calls)).toBe(1);
  });

  it('shows the second step and the join date of the member, and never asks for every team', async () => {
    const { page, calls } = await open(`/org/members/${HANA}`, { secondFactorRule: 'everyone' });
    const facts = await within(page).findByRole('region', { name: Ar['shell.crumb.member'] });
    expect(await within(facts).findByText(Ar['members.mfa.none'])).toBeInTheDocument();
    expect(calls.filter((call) => new URL(call.url).pathname.endsWith('/teams'))).toEqual([]);
    expect(facts).toHaveTextContent(Ar['members.mfa.needs']);
    expect(facts).toHaveTextContent(Ar['member.facts.joined']);
  });

  it('shows when the member was last active, or that no activity is recorded', async () => {
    const { page, user } = await open(`/org/members/${HANA}`);
    const facts = await within(page).findByRole('region', { name: Ar['shell.crumb.member'] });
    expect(await within(facts).findByText(Ar['member.facts.lastActive'])).toBeInTheDocument();
    expect(facts.querySelector('time[datetime="2026-10-28T09:30:00Z"]')).not.toBeNull();
    expect(facts).not.toHaveTextContent(Ar['member.facts.lastActive.none']);

    await user.click(linkTo(await listOf(page), 'Mona Adel'));
    await within(page).findByRole('heading', { level: 2, name: 'Mona Adel' });
    expect(within(page).getByRole('region', { name: Ar['shell.crumb.member'] })).toHaveTextContent(Ar['member.facts.lastActive.none']);
  });

  it('shows last activity to people who manage members only', async () => {
    const { page } = await open(`/org/members/${HANA}`, { role: 'developer' });
    const facts = await within(page).findByRole('region', { name: Ar['shell.crumb.member'] });
    await within(facts).findByText(Ar['member.facts.joined']);
    expect(facts).not.toHaveTextContent(Ar['member.facts.lastActive']);
  });

  it('shows the not-found state for a member who is not in the organization', async () => {
    const { page } = await open('/org/members/0198a000-0000-7000-8000-0000000000f9');
    expect(await within(page).findByText(Ar['member.error'])).toBeInTheDocument();
  });

  it('offers a retry when the members cannot be loaded', async () => {
    let failing = true;
    const { page, user } = await open(`/org/members/${HANA}`, {
      routes: [
        (request, url) =>
          failing && request.method === 'GET' && url.pathname.endsWith('/members') ? backendFailure('INTERNAL', 'x', 500) : undefined,
      ],
    });
    const detail = await within(page).findByRole('region', { name: Ar['shell.crumb.member'] });
    expect(await within(detail).findByText(Ar['members.error'])).toBeInTheDocument();
    failing = false;
    await user.click(within(detail).getByRole('button', { name: Ar['status.retry'] }));
    expect(await within(page).findByRole('heading', { level: 2, name: 'Hana Samir' })).toBeInTheDocument();
  });

  it('isolates names in the list so an Arabic name keeps its place in English text', async () => {
    const members = [
      ...membersFixture(),
      { UserId: '0198a000-0000-7000-8000-0000000000b5', Name: 'صبري داود', Email: 'sabry2@future.dev', Role: 'viewer', EffectiveRole: 'viewer', JoinedAt: '2026-10-08T08:00:00Z', Mfa: 'totp', Teams: [] },
    ];
    const { page } = await open('/org/members', { members });
    const list = await listOf(page);
    expect(within(list).getByText('صبري داود').tagName).toBe('BDI');
    expect(within(list).getByText('sabry2@future.dev').tagName).toBe('BDI');
  });
});

describe('removing a member', () => {
  it('asks in a slide and removes only after the confirmation', async () => {
    const { page, user, calls, router } = await open(`/org/members/${HANA}`);
    await user.click(await within(page).findByRole('button', { name: Ar['member.remove.action'].replace('{name}', isolate('Hana Samir')) }));
    const slide = await screen.findByRole('dialog', { name: Ar['member.remove.title'] });
    expect(router.state.location.search).toEqual({ slide: 'member-remove' });
    expect(calls.some((call) => call.method === 'DELETE')).toBe(false);

    await user.click(await within(slide).findByRole('button', { name: Ar['member.cancel'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(calls.some((call) => call.method === 'DELETE')).toBe(false);

    await user.click(within(page).getByRole('button', { name: Ar['member.remove.action'].replace('{name}', isolate('Hana Samir')) }));
    const again = await screen.findByRole('dialog', { name: Ar['member.remove.title'] });
    await user.click(await within(again).findByRole('button', { name: Ar['member.remove.confirm'] }));
    await waitFor(() => {
      expect(calls.some((call) => call.method === 'DELETE' && call.url.endsWith(`/members/${HANA}`))).toBe(true);
    });
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/org/members');
    });
  });

  it('refuses the slide for a member the actor cannot remove', async () => {
    const members = membersFixture().map((member) => (member.UserId === MONA ? { ...member, Role: 'owner', EffectiveRole: 'owner' } : member));
    await open(`/org/members/${MONA}?slide=member-remove`, { role: 'admin', members });
    expect(await screen.findByText(Ar['member.remove.unavailable'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['member.remove.confirm'] })).not.toBeInTheDocument();
  });
});

describe('member actions per role', () => {
  it.each([
    ['owner', true],
    ['admin', true],
    ['maintainer', false],
    ['developer', false],
  ] as const)('as a %s, can manage another member: %s', async (role, manage) => {
    const { page } = await open(`/org/members/${HANA}`, { role });
    await within(page).findByRole('heading', { level: 2, name: 'Hana Samir' });
    const remove = within(page).queryByRole('button', { name: Ar['member.remove.action'].replace('{name}', isolate('Hana Samir')) });
    const roleSelect = within(page).queryByRole('combobox', { name: Ar['member.role.pick'] });
    const invite = within(page).queryByRole('button', { name: Ar['organization.invite'] });
    for (const control of [remove, roleSelect, invite]) {
      if (manage) expect(control).toBeInTheDocument();
      else expect(control).not.toBeInTheDocument();
    }
    expect(within(page).queryAllByText(Ar['member.facts.mfa']).length > 0).toBe(manage);
  });

  it('keeps an admin away from the role and the removal of an owner', async () => {
    const members = membersFixture().map((member) => (member.UserId === MONA ? { ...member, Role: 'owner', EffectiveRole: 'owner' } : member));
    const { page } = await open(`/org/members/${MONA}`, { role: 'admin', members });
    await within(page).findByRole('heading', { level: 2, name: 'Mona Adel' });
    expect(within(page).queryByRole('combobox', { name: Ar['member.role.pick'] })).not.toBeInTheDocument();
    expect(within(page).queryByRole('button', { name: Ar['member.remove.action'].replace('{name}', isolate('Mona Adel')) })).not.toBeInTheDocument();
    expect(within(page).queryByRole('button', { name: Ar['member.mfa.reset'] })).not.toBeInTheDocument();
  });

  it('is not a page for a viewer', async () => {
    startBackend({ role: 'viewer' });
    const router = createAppRouter(createMemoryHistory({ initialEntries: [`/org/members/${HANA}`] }));
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
  });
});

describe("a member's teams", () => {
  it('lists the teams from the members list, without a request per team, and opens one', async () => {
    const { page, user, router, calls } = await open(`/org/members/${HANA}`);
    await within(page).findByRole('heading', { level: 2, name: 'Hana Samir' });
    const facts = within(page).getByRole('region', { name: Ar['shell.crumb.member'] });
    const row = within(facts).getByText(Ar['member.facts.teams']).closest('div') as HTMLElement;
    const link = within(row).getByRole('link', { name: 'Backend' });
    expect(link).toHaveAttribute('href', `/org/teams/${TEAM_ID}`);
    expect(within(link).getByText('Backend').tagName).toBe('BDI');
    expect(calls.filter((call) => new URL(call.url).pathname.includes('/teams'))).toEqual([]);

    await user.click(link);
    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/org/teams/${TEAM_ID}`);
    });
  });

  it('says so when the member has no team', async () => {
    const { page } = await open(`/org/members/${MONA}`);
    await within(page).findByRole('heading', { level: 2, name: 'Mona Adel' });
    const facts = within(page).getByRole('region', { name: Ar['shell.crumb.member'] });
    const row = within(facts).getByText(Ar['member.facts.teams']).closest('div') as HTMLElement;
    expect(within(row).getByText(Ar['member.facts.teams.none'])).toBeInTheDocument();
    expect(within(row).queryByRole('link')).not.toBeInTheDocument();
  });
});

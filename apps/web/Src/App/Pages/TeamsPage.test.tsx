import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope, ORG_ID } from '@/Core/Testing/Fetch';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

configure({ asyncUtilTimeout: 5000 });

const BACKEND = '0198a000-0000-7000-8000-0000000000d1';
const DESIGN = '0198a000-0000-7000-8000-0000000000d3';
const TEAMS_PATH = `/api/v1/organizations/${ORG_ID}/teams`;

const team = (Id: string, Name: string, members: { UserId: string; Name: string }[]) => ({
  Id,
  Name,
  Description: '',
  MemberCount: members.length,
  CreatedAt: '2026-10-20T08:00:00Z',
  Bindings: [],
  Members: members.map((member) => ({ ...member, Email: 'x@future.dev', Role: 'developer', AddedAt: '2026-10-21T08:00:00Z' })),
});

const TEAMS = [
  team(BACKEND, 'Backend', [{ UserId: '0198a000-0000-7000-8000-0000000000b3', Name: 'Hana Samir' }]),
  team(DESIGN, 'فريق التصميم', []),
];

const teamsRoute =
  (teams = TEAMS): TRouteHandler =>
  (request, url) => {
    if (request.method !== 'GET' || !url.pathname.startsWith(TEAMS_PATH)) return undefined;
    if (url.pathname === TEAMS_PATH) return envelope({ Success: true, Data: teams.map((item) => ({ ...item, Members: undefined })) });
    const found = teams.find((item) => url.pathname.endsWith(item.Id));
    return found ? envelope({ Success: true, Data: found }) : backendFailure('NOT_FOUND', 'x', 404);
  };

const open = async (url: string, role = 'owner', routes: TRouteHandler[] = [teamsRoute()]) => {
  const backend = startBackend({ role, routes });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const page = await findPage(Ar['shell.nav.teams']);
  return { ...backend, router, user, page };
};

const listOf = (page: HTMLElement) => within(page).findByRole('list', { name: Ar['teams.title'] });

const teamListRequests = (calls: readonly Request[]) =>
  calls.filter((call) => call.method === 'GET' && new URL(call.url).pathname === TEAMS_PATH).length;

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the teams page', () => {
  it('puts the team beside the list and keeps the list when switching teams', async () => {
    const { page, user, calls } = await open(`/org/teams/${BACKEND}`);
    const list = await listOf(page);
    expect(await within(page).findByRole('list', { name: Ar['team.members.title'] })).toHaveTextContent('Hana Samir');
    expect(page.querySelector('[data-master-list]')).toHaveClass('max-xl:hidden');
    expect(within(page).getByRole('link', { name: Ar['teams.back'] })).toHaveClass('xl:hidden');
    expect(within(list).getByRole('link', { current: 'page' })).toHaveTextContent('Backend');

    const design = within(list).getByText('فريق التصميم').closest('a');
    if (design === null) throw new Error('no link for the design team');
    await user.click(design);
    expect(await within(page).findByText(Ar['team.members.emptyManager'])).toBeInTheDocument();
    expect(await listOf(page)).toBe(list);
    expect(within(list).getByRole('link', { current: 'page' })).toHaveTextContent('فريق التصميم');
    expect(teamListRequests(calls)).toBe(1);
  });

  it('isolates team names in the list', async () => {
    const { page } = await open('/org/teams');
    const list = await listOf(page);
    expect(within(list).getByText('فريق التصميم').tagName).toBe('BDI');
    expect(page.querySelector('[data-master-detail]')).toBeNull();
  });

  it('offers a new team from the empty state to those who may create one', async () => {
    const { page } = await open('/org/teams', 'admin', [teamsRoute([])]);
    expect(await within(page).findByText(Ar['teams.empty'])).toBeInTheDocument();
    expect(within(page).getAllByRole('button', { name: Ar['teams.new'] })).toHaveLength(2);
  });

  it('shows an error with a retry when the teams cannot be loaded', async () => {
    const failing: TRouteHandler = (request, url) =>
      request.method === 'GET' && url.pathname === TEAMS_PATH ? backendFailure('INTERNAL', 'x', 500) : undefined;
    const { page } = await open('/org/teams', 'owner', [failing]);
    expect(await within(page).findByText(Ar['teams.error'])).toBeInTheDocument();
    expect(within(page).getByRole('button', { name: Ar['status.retry'] })).toBeInTheDocument();
  });

  it('says when the team does not exist', async () => {
    const { page } = await open('/org/teams/0198a000-0000-7000-8000-0000000000d9');
    expect(await within(page).findByText(Ar['team.error'])).toBeInTheDocument();
  });
});

describe('team actions per role', () => {
  it.each([
    ['owner', true],
    ['admin', true],
    ['maintainer', false],
    ['developer', false],
  ] as const)('as a %s, can manage the team: %s', async (role, manage) => {
    const { page } = await open(`/org/teams/${BACKEND}`, role);
    await within(page).findByRole('list', { name: Ar['team.members.title'] });
    const controls = [
      within(page).queryByRole('button', { name: Ar['teams.new'] }),
      within(page).queryByRole('combobox', { name: Ar['team.role.pick'] }),
      within(page).queryByRole('button', { name: Ar['team.members.add'] }),
      within(page).queryByRole('button', { name: Ar['team.members.remove'].replace('{name}', isolate('Hana Samir')) }),
      within(page).queryByRole('button', { name: Ar['team.delete'] }),
      within(page).queryByLabelText(Ar['team.name']),
    ];
    for (const control of controls) {
      if (manage) expect(control).toBeInTheDocument();
      else expect(control).not.toBeInTheDocument();
    }
  });

  it('is not a page for a viewer', async () => {
    startBackend({ role: 'viewer', routes: [teamsRoute()] });
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: [`/org/teams/${BACKEND}`] }))} queryClient={createQueryClient({ retry: false })} />);
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
  });
});

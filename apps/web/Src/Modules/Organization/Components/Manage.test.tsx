import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { formatDateTime } from '@/Core/Format/TimeFormat';
import { useOrganizationStore } from '@/Core/Store/useOrganizationStore';
import { useToastStore } from '@/Core/Store/useToastStore';
import { membersFixture, startBackend, type IBackendOptions } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID } from '@/Core/Testing/Fetch';
import { INVITE_LINK, orgBackend, TEAM_ID } from '@/Core/Testing/OrgBackend';
import { findPage } from '@/Core/Testing/Pages';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const HANA = '0198a000-0000-7000-8000-0000000000b3';
const MONA = '0198a000-0000-7000-8000-0000000000b2';
const OTHER_ORG = '0198a000-0000-7000-8000-0000000000a2';

const open = async (url: string, title: string, options: IBackendOptions = {}, emailStatus?: string) => {
  const org = orgBackend(emailStatus);
  const backend = startBackend({ ...options, routes: [...(options.routes ?? []), ...org.routes] });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = url.includes('?slide=') ? await screen.findByRole('dialog', { name: title }, { timeout: 5000 }) : await findPage(title);
  return { ...backend, org, router, user, dialog };
};

const bodyOf = async (calls: Request[], method: string, suffix: string): Promise<unknown> => {
  const call = calls.find((request) => request.method === method && new URL(request.url).pathname.endsWith(suffix));
  return call ? ((await call.clone().json()) as unknown) : undefined;
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
  useOrganizationStore.setState({ CurrentId: null });
});

describe('inviting a member', () => {
  it('offers only roles up to the inviter, then shows the link once and says no email went out', async () => {
    const { user, dialog, calls } = await open('/org/members?slide=organization-invite', Ar['invite.title'], { role: 'admin' });
    const roles = within(dialog).getByRole('radiogroup', { name: Ar['invite.role'] });
    expect(within(roles).queryByRole('radio', { name: new RegExp(Ar['role.owner']) })).not.toBeInTheDocument();
    expect(within(roles).getAllByRole('radio')).toHaveLength(4);
    await user.type(within(dialog).getByLabelText(Ar['invite.email']), 'new@future.dev');
    await user.click(within(roles).getByRole('radio', { name: new RegExp(Ar['role.viewer']) }));
    await user.click(within(dialog).getByRole('button', { name: Ar['invite.create'] }));
    expect(await within(dialog).findByText(INVITE_LINK)).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['invite.email.disabled'])).toBeInTheDocument();
    expect(await bodyOf(calls, 'POST', '/invitations')).toEqual({ Email: 'new@future.dev', Role: 'viewer' });
  });

  it('refuses a malformed email without calling the server', async () => {
    const { user, dialog, calls } = await open('/org/members?slide=organization-invite', Ar['invite.title']);
    await user.type(within(dialog).getByLabelText(Ar['invite.email']), 'not-an-email');
    await user.click(within(dialog).getByRole('button', { name: Ar['invite.create'] }));
    expect(await within(dialog).findByText(Ar['form.error.email'])).toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST')).toBe(false);
  });

  it('says when the email was sent', async () => {
    const { user, dialog } = await open('/org/members?slide=organization-invite', Ar['invite.title'], {}, 'sent');
    await user.type(within(dialog).getByLabelText(Ar['invite.email']), 'new@future.dev');
    await user.click(within(dialog).getByRole('button', { name: Ar['invite.create'] }));
    expect(await within(dialog).findByText(Ar['invite.email.sent'].replace('{email}', isolate('new@future.dev')))).toBeInTheDocument();
  });

  it('lists open invitations, revokes one and makes a new link for another', async () => {
    const { user, dialog, calls } = await open('/org/invitations', Ar['shell.nav.invitations']);
    const list = await within(dialog).findByRole('list', { name: Ar['invitations.title'] });
    expect(list).toHaveTextContent('ali@future.dev');
    expect(list).toHaveTextContent(Ar['invitations.expires'].replace('{when}', isolate(formatDateTime('2026-11-03T08:00:00Z', 'ar'))));
    await user.click(within(list).getByRole('button', { name: Ar['invitations.resendLabel'].replace('{email}', isolate('ali@future.dev')) }));
    expect(await within(dialog).findByText(INVITE_LINK)).toBeInTheDocument();
    await user.click(within(list).getByRole('button', { name: Ar['invitations.revokeLabel'].replace('{email}', isolate('ali@future.dev')) }));
    expect(await within(dialog).findByText(Ar['invitations.empty'])).toBeInTheDocument();
    expect(calls.some((call) => call.method === 'DELETE' && call.url.includes('/invitations/'))).toBe(true);
  });
});

describe('a member page', () => {
  it('changes a role and says the sessions ended', async () => {
    const { user, dialog, calls } = await open(`/org/members/${HANA}`, Ar['shell.nav.members']);
    const role = await within(dialog).findByRole('combobox', { name: Ar['member.role.pick'] });
    await user.selectOptions(role, 'maintainer');
    await user.click(within(dialog).getByRole('button', { name: Ar['member.role.save'] }));
    await waitFor(() => {
      expect(useToastStore.getState().Toasts.some((toast) => toast.Title.includes('Hana Samir'))).toBe(true);
    });
    expect(await bodyOf(calls, 'PATCH', `/members/${HANA}`)).toEqual({ Role: 'maintainer' });
  });

  it('asks for a current factor before granting the owner role', async () => {
    const { user, dialog, calls } = await open(`/org/members/${HANA}`, Ar['shell.nav.members']);
    await user.selectOptions(await within(dialog).findByRole('combobox', { name: Ar['member.role.pick'] }), 'owner');
    await user.click(within(dialog).getByRole('button', { name: Ar['member.role.save'] }));
    await user.type(await within(dialog).findByLabelText(Ar['factor.method.password']), 'correct horse battery staple');
    await user.click(within(dialog).getAllByRole('button', { name: Ar['member.role.save'] }).at(-1) ?? dialog);
    await waitFor(async () => {
      expect(await bodyOf(calls, 'PATCH', `/members/${HANA}`)).toEqual({
        Role: 'owner',
        CurrentFactor: { Method: 'password', Password: 'correct horse battery staple' },
      });
    });
  });

  it('resets a lost second factor after the actor confirms', async () => {
    const { user, dialog, calls } = await open(`/org/members/${MONA}`, Ar['shell.nav.members']);
    await user.click(await within(dialog).findByRole('button', { name: Ar['member.mfa.reset'] }));
    await user.type(await within(dialog).findByLabelText(Ar['factor.method.password']), 'correct horse battery staple');
    await user.click(within(dialog).getAllByRole('button', { name: Ar['member.mfa.reset'] }).at(-1) ?? dialog);
    await waitFor(async () => {
      expect(await bodyOf(calls, 'POST', `/members/${MONA}/mfa-reset`)).toEqual({
        CurrentFactor: { Method: 'password', Password: 'correct horse battery staple' },
      });
    });
  });

  it('sends an owner through the role change before removing them', async () => {
    const coOwner = '0198a000-0000-7000-8000-0000000000b4';
    const members = [
      ...membersFixture(),
      { UserId: coOwner, Name: 'Laila Owner', Email: 'laila@future.dev', Role: 'owner', EffectiveRole: 'owner', JoinedAt: '2026-10-06T08:00:00Z', Mfa: 'totp', Teams: [] },
    ];
    const { dialog } = await open(`/org/members/${coOwner}`, Ar['shell.nav.members'], { members });
    expect(await within(dialog).findByText(Ar['member.remove.ownerFirst'].replace('{name}', isolate('Laila Owner')))).toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: Ar['member.remove.action'].replace('{name}', isolate('Laila Owner')) })).not.toBeInTheDocument();
  });

  it('points to the security page on your own member page', async () => {
    const owner = '0198a000-0000-7000-8000-0000000000b1';
    const { user, dialog, router } = await open(`/org/members/${owner}`, Ar['shell.nav.members'], { role: 'admin' });
    expect(await within(dialog).findByText(Ar['member.self'])).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: Ar['member.self.open'] }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/account/security');
    });
  });
});

describe('teams', () => {
  it('opens a team, raises its role and reports the ended sessions', async () => {
    const { user, dialog, calls } = await open(`/org/teams/${TEAM_ID}`, Ar['shell.nav.teams']);
    const role = await within(dialog).findByRole('combobox', { name: Ar['team.role.pick'] });
    expect(within(dialog).getByRole('list', { name: Ar['team.members.title'] })).toHaveTextContent('Hana Samir');
    await user.selectOptions(role, 'admin');
    await user.click(within(dialog).getByRole('button', { name: Ar['team.role.save'] }));
    await waitFor(() => {
      expect(useToastStore.getState().Toasts.some((toast) => toast.Detail === arText('team.revoked', { count: 1 }))).toBe(true);
    });
    expect(await bodyOf(calls, 'PATCH', '/role-bindings/0198a000-0000-7000-8000-0000000000d2')).toEqual({ Role: 'admin' });
  });

  it('adds a member picked from the organization', async () => {
    const { user, dialog, calls } = await open(`/org/teams/${TEAM_ID}`, Ar['shell.nav.teams']);
    await user.click(await within(dialog).findByRole('button', { name: Ar['team.members.add'] }));
    const pick = await within(dialog).findByRole('combobox', { name: Ar['team.members.pick'] });
    await user.selectOptions(pick, MONA);
    await user.click(within(dialog).getByRole('button', { name: Ar['team.members.addPicked'] }));
    await waitFor(async () => {
      expect(await bodyOf(calls, 'POST', `/teams/${TEAM_ID}/members`)).toEqual({ UserId: MONA });
    });
  });

  it('lists teams with their organization role', async () => {
    const { dialog } = await open('/org/teams', Ar['shell.nav.teams']);
    const list = await within(dialog).findByRole('list', { name: Ar['teams.title'] });
    expect(list).toHaveTextContent('Backend');
    expect(list).toHaveTextContent(Ar['teams.roleIn'].replace('{role}', isolate(Ar['role.developer'])));
  });
});

describe('switching and leaving organizations', () => {
  const twoOrgs = (): IBackendOptions => ({
    routes: [
      (request, url) => {
        if (!url.pathname.endsWith('/me') || request.method !== 'GET') return undefined;
        const me = meBody();
        me.Data.Memberships.push({ OrganizationId: OTHER_ORG, OrganizationName: 'Sweets Store', OrganizationSlug: 'sweets', Role: 'viewer' });
        return envelope(me);
      },
    ],
  });

  it('switches through the organization chip and remembers the choice', async () => {
    const { user, dialog } = await open('/organizations', Ar['organizations.title'], twoOrgs());
    expect(within(dialog).getByText(Ar['organizations.current'])).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: /Sweets Store/ }));
    await waitFor(() => {
      expect(useOrganizationStore.getState().CurrentId).toBe(OTHER_ORG);
    });
    expect(
      (await screen.findAllByRole('button', { name: Ar['shell.organization.switch'].replace('{name}', isolate('Sweets Store')) })).length,
    ).toBeGreaterThan(0);
  });

  it('leaves after a confirmation slide', async () => {
    const { user, dialog, calls, router } = await open('/org/settings', Ar['shell.nav.orgSettings'], { role: 'developer' });
    await user.click(await within(dialog).findByRole('button', { name: Ar['organization.leave.action'].replace('{name}', isolate('Future Solution')) }));
    const slide = await screen.findByRole('dialog', { name: Ar['organization.leave.title'] });
    expect(calls.some((call) => call.method === 'POST' && call.url.endsWith('/leave'))).toBe(false);
    await user.click(await within(slide).findByRole('button', { name: Ar['organization.leave.confirm'] }));
    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST' && call.url.endsWith(`/organizations/${ORG_ID}/leave`))).toBe(true);
    });
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/');
    });
  });

  it('hands the ownership over with a factor', async () => {
    const { user, dialog, calls } = await open('/org/settings?slide=organization-transfer', Ar['organization.transfer.title']);
    await user.click(await within(dialog).findByRole('radio', { name: /Mona Adel/ }));
    await user.type(await within(dialog).findByLabelText(Ar['factor.method.password']), 'correct horse battery staple');
    await user.click(within(dialog).getByRole('button', { name: Ar['organization.transfer.action'] }));
    await waitFor(async () => {
      expect(await bodyOf(calls, 'POST', '/transfer-ownership')).toEqual({
        UserId: MONA,
        CurrentFactor: { Method: 'password', Password: 'correct horse battery staple' },
      });
    });
  });
});

describe('the second-step reset banner', () => {
  it('asks the member to set up a new factor', async () => {
    const backend = startBackend({
      routes: [
        (request, url) => {
          if (!url.pathname.endsWith('/me') || request.method !== 'GET') return undefined;
          const me = meBody();
          me.Data.MfaReenrollRequired = true;
          return envelope(me);
        },
      ],
    });
    const router = createAppRouter(createMemoryHistory({ initialEntries: ['/'] }));
    const user = userEvent.setup();
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    expect(await screen.findByText(Ar['mfaReset.banner.title'])).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['mfaReset.banner.action'] }));
    expect(await findPage(Ar['shell.nav.security'])).toBeInTheDocument();
    expect(backend.state.signedIn).toBe(true);
  });
});

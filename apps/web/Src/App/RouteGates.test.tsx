import { createMemoryHistory } from '@tanstack/react-router';
import { configure, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend } from '@/Core/Testing/Backend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const ID = '0198a000-0000-7000-8000-0000000000e1';
const ROLES = ['owner', 'admin', 'maintainer', 'developer', 'viewer'] as const;
type TRoleName = (typeof ROLES)[number];

interface IGateCase {
  url: string;
  title: string;
  endpoint: RegExp;
  allowed: readonly TRoleName[];
}

const EVERYONE = ROLES;
const BUILDERS: readonly TRoleName[] = ['owner', 'admin', 'maintainer', 'developer'];
const ADMINS: readonly TRoleName[] = ['owner', 'admin'];

const GATES: readonly IGateCase[] = [
  { url: '/jobs', title: Ar['jobs.title'], endpoint: /\/jobs/, allowed: EVERYONE },
  { url: `/jobs/${ID}`, title: Ar['jobs.detail.title'], endpoint: /\/jobs\//, allowed: EVERYONE },
  { url: '/approvals', title: Ar['approvals.page.title'], endpoint: /\/deployment-approvals/, allowed: BUILDERS },
  { url: '/runners', title: Ar['runners.title'], endpoint: /\/runners/, allowed: BUILDERS },
  { url: `/runners/${ID}`, title: Ar['runners.title'], endpoint: /\/runners\//, allowed: BUILDERS },
  { url: '/runners/tokens', title: Ar['runners.tokens.title'], endpoint: /\/runner-registration-tokens/, allowed: ADMINS },
  { url: '/org/members', title: Ar['shell.nav.members'], endpoint: /\/members/, allowed: BUILDERS },
  { url: `/org/members/${ID}`, title: Ar['shell.nav.members'], endpoint: /\/members/, allowed: BUILDERS },
  { url: '/org/teams', title: Ar['shell.nav.teams'], endpoint: /\/teams/, allowed: BUILDERS },
  { url: `/org/teams/${ID}`, title: Ar['shell.nav.teams'], endpoint: /\/teams/, allowed: BUILDERS },
  { url: '/org/invitations', title: Ar['shell.nav.invitations'], endpoint: /\/invitations/, allowed: ADMINS },
  { url: '/org/api-keys', title: Ar['shell.nav.apiKeys'], endpoint: /\/organizations\/[^/]+\/api-tokens/, allowed: ADMINS },
  { url: '/org/secrets', title: Ar['shell.nav.secrets'], endpoint: /\/organizations\/[^/]+\/secrets/, allowed: BUILDERS },
  { url: '/org/settings', title: Ar['shell.nav.orgSettings'], endpoint: /^$/, allowed: EVERYONE },
  { url: '/audit', title: Ar['audit.title'], endpoint: /\/audit-events/, allowed: ADMINS },
  { url: `/audit/${ID}`, title: Ar['audit.detail.title'], endpoint: /\/audit-events/, allowed: ADMINS },
];

const CASES = GATES.flatMap((gate) => ROLES.map((role) => ({ ...gate, role, open: gate.allowed.includes(role) })));

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('who can open each page', () => {
  it.each(CASES)('$url for a $role: open=$open', async ({ url, title, endpoint, role, open }) => {
    const { calls } = startBackend({ role });
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: [url] }))} queryClient={createQueryClient({ retry: false })} />);
    if (open) {
      expect(await screen.findByRole('heading', { level: 1, name: title })).toBeInTheDocument();
      expect(screen.queryByText(Ar['notFound.text'])).not.toBeInTheDocument();
      return;
    }
    expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
    expect(screen.queryByRole('heading', { level: 1, name: title })).not.toBeInTheDocument();
    expect(calls.map((call) => new URL(call.url).pathname).filter((path) => endpoint.test(path))).toEqual([]);
  });
});

describe('ids in the address', () => {
  it.each(['/jobs/42', '/runners/abc', `/org/members/${ID.toUpperCase()}`, '/org/teams/x', '/audit/1', '/jobs/42/more'])(
    'shows the not-found state for %s without asking the server',
    async (url) => {
      const { calls } = startBackend({ role: 'owner' });
      render(<App router={createAppRouter(createMemoryHistory({ initialEntries: [url] }))} queryClient={createQueryClient({ retry: false })} />);
      expect(await screen.findByText(Ar['notFound.text'])).toBeInTheDocument();
      expect(calls.filter((call) => /\/(jobs|runners|teams|audit-events)\//.test(new URL(call.url).pathname))).toEqual([]);
    },
  );
});

describe('slides the role cannot use', () => {
  it.each([
    ['viewer', '/org/settings?slide=organization-transfer'],
    ['admin', '/org/settings?slide=organization-transfer'],
    ['developer', '/runners?slide=runner-new'],
    ['developer', '/org/teams?slide=team-new'],
    ['maintainer', '/org/members?slide=organization-invite'],
    ['developer', `/org/members/${ID}?slide=member-remove`],
    ['viewer', '/org/settings?slide=member-remove'],
  ])('refuses them for a %s at %s and keeps the page', async (role, url) => {
    startBackend({ role });
    const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    expect(await screen.findByText(Ar['toast.unknownPanel.title'])).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});

import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useToastStore } from '@/Core/Store/useToastStore';
import { startBackend } from '@/Core/Testing/Backend';
import { Ar } from '@/I18n/Locales/Ar';

const ID = '0198a000-0000-7000-8000-0000000000e1';

const mount = async (role: string, url = '/') => {
  startBackend({ role });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const nav = await screen.findByRole('navigation', { name: Ar['shell.nav.label'] }, { timeout: 5000 });
  return { router, user, nav };
};

const groupLinks = (nav: HTMLElement, label: string): (string | null)[] =>
  within(within(nav).getByRole('group', { name: label }))
    .getAllByRole('link')
    .map((link) => link.textContent);

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

const ADMIN_MENU = {
  run: [Ar['shell.nav.projects'], Ar['shell.nav.jobs'], Ar['shell.nav.runners']],
  org: [Ar['shell.nav.members'], Ar['shell.nav.teams'], Ar['shell.nav.invitations'], Ar['shell.nav.apiKeys'], Ar['shell.nav.orgSettings']],
  watch: [Ar['shell.nav.audit'], Ar['shell.nav.system']],
};
const BUILDER_MENU = {
  run: [Ar['shell.nav.projects'], Ar['shell.nav.jobs'], Ar['shell.nav.runners']],
  org: [Ar['shell.nav.members'], Ar['shell.nav.teams'], Ar['shell.nav.orgSettings']],
  watch: [Ar['shell.nav.system']],
};

describe('the side menu', () => {
  it.each([
    ['owner', ADMIN_MENU],
    ['admin', ADMIN_MENU],
    ['maintainer', BUILDER_MENU],
    ['developer', BUILDER_MENU],
    ['viewer', { run: [Ar['shell.nav.projects'], Ar['shell.nav.jobs']], org: [Ar['shell.nav.orgSettings']], watch: [Ar['shell.nav.system']] }],
  ])('groups the sections for a %s and hides what the role cannot open', async (role, menu) => {
    const { nav } = await mount(role);
    expect(groupLinks(nav, Ar['shell.group.run'])).toEqual(menu.run);
    expect(groupLinks(nav, Ar['shell.group.org'])).toEqual(menu.org);
    expect(groupLinks(nav, Ar['shell.group.watch'])).toEqual(menu.watch);
    const groups = within(nav).getAllByRole('group').map((group) => group.getAttribute('data-menu-group'));
    expect(groups).toEqual(['run', 'org', 'watch']);
    expect(within(nav).getAllByRole('link')[0]).toHaveTextContent(Ar['shell.nav.home']);
    expect(within(nav).queryByRole('button', { name: /المشاريع/ })).not.toBeInTheDocument();
  });

  it.each([
    ['/', Ar['shell.nav.home']],
    ['/jobs', Ar['shell.nav.jobs']],
    [`/jobs/${ID}`, Ar['shell.nav.jobs']],
    ['/runners/tokens', Ar['shell.nav.runners']],
    [`/org/teams/${ID}`, Ar['shell.nav.teams']],
    ['/system/checks/database', Ar['shell.nav.system']],
  ])('marks the section of %s as the current page', async (url, label) => {
    const { nav } = await mount('owner', url);
    const current = within(nav)
      .getAllByRole('link')
      .filter((link) => link.getAttribute('aria-current') === 'page');
    expect(current.map((link) => link.textContent)).toEqual([label]);
  });

  it('marks the account row on an account page and opens a section in place', async () => {
    const { nav, user, router } = await mount('owner', '/account/security');
    expect(within(nav).getByRole('link', { name: /Sabry Dawood/ })).toHaveAttribute('aria-current', 'page');
    await user.click(within(nav).getByRole('link', { name: Ar['shell.nav.runners'] }));
    expect(await screen.findByRole('heading', { level: 1, name: Ar['runners.title'] }, { timeout: 5000 })).toBeInTheDocument();
    expect(router.state.location.href).toBe('/runners');
  });
});

describe('the top bar path', () => {
  it.each([
    ['/jobs', [Ar['shell.group.run'], Ar['shell.nav.jobs']]],
    [`/jobs/${ID}`, [Ar['shell.group.run'], Ar['shell.nav.jobs'], Ar['jobs.detail.title']]],
    ['/runners/tokens', [Ar['shell.group.run'], Ar['shell.nav.runners'], Ar['shell.nav.runnerTokens']]],
    [`/org/members/${ID}`, [Ar['shell.group.org'], Ar['shell.nav.members'], Ar['shell.crumb.member']]],
    ['/account/preferences', [Ar['shell.group.account'], Ar['shell.nav.preferences']]],
    ['/', [Ar['shell.nav.home']]],
  ])('reads the path of %s', async (url, crumbs) => {
    await mount('owner', url);
    const nav = screen.getByRole('navigation', { name: Ar['shell.crumbs.label'] });
    expect(within(nav).getAllByRole('listitem').map((item) => item.textContent)).toEqual(crumbs);
  });
});

describe('the phone bar', () => {
  it('has six sections, each with an icon and its name, and opens the organization on its members', async () => {
    const { router, user } = await mount('owner', '/jobs');
    const bar = screen.getByRole('navigation', { name: Ar['shell.bottom.label'] });
    const links = within(bar).getAllByRole('link');
    expect(links.map((link) => link.textContent)).toEqual([
      Ar['shell.nav.home'],
      Ar['shell.nav.projects'],
      Ar['shell.nav.jobs'],
      Ar['shell.nav.runners'],
      Ar['shell.bottom.org'],
      Ar['shell.group.account'],
    ]);
    for (const link of links) expect(link.querySelector('svg')).not.toBeNull();
    expect(within(bar).getByRole('link', { name: Ar['shell.nav.jobs'] })).toHaveAttribute('aria-current', 'page');
    expect(within(bar).getByRole('link', { name: Ar['shell.bottom.org'] })).toHaveAttribute('href', '/org/members');
    await user.click(within(bar).getByRole('link', { name: Ar['shell.bottom.org'] }));
    expect(await screen.findByRole('heading', { level: 1, name: Ar['shell.nav.members'] }, { timeout: 5000 })).toBeInTheDocument();
    expect(router.state.location.href).toBe('/org/members');
    expect(within(bar).getByRole('link', { name: Ar['shell.bottom.org'] })).toHaveAttribute('aria-current', 'page');
  });

  it('drops runners for a viewer and sends the organization item to the settings', async () => {
    await mount('viewer', '/account/profile');
    const bar = screen.getByRole('navigation', { name: Ar['shell.bottom.label'] });
    expect(within(bar).getAllByRole('link').map((link) => link.textContent)).toEqual([
      Ar['shell.nav.home'],
      Ar['shell.nav.projects'],
      Ar['shell.nav.jobs'],
      Ar['shell.bottom.org'],
      Ar['shell.group.account'],
    ]);
    expect(within(bar).getByRole('link', { name: Ar['shell.bottom.org'] })).toHaveAttribute('href', '/org/settings');
    expect(within(bar).getByRole('link', { name: Ar['shell.group.account'] })).toHaveAttribute('aria-current', 'page');
  });

  it('shows the organization section links on organization pages', async () => {
    await mount('admin', '/org/teams');
    const sections = await screen.findByRole('navigation', { name: Ar['shell.sections.org'] });
    expect(sections).toHaveClass('md:hidden');
    expect(within(sections).getByRole('link', { name: Ar['shell.nav.teams'] })).toHaveAttribute('aria-current', 'page');
  });
});

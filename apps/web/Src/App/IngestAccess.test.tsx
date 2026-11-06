import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { newIngestState, OTHER_TOKEN_ID, TOKEN_ID } from '@/Core/Testing/IngestBackend';
import { API, mount, PAGE_URL, PASSWORD, PROJECT_NAME } from '@/Core/Testing/IngestHarness';
import { findPage } from '@/Core/Testing/Pages';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('revoking a token', () => {
  it('asks for a confirmation and the second step, then marks the token revoked', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    await view.user.click(await within(main).findByRole('button', { name: arText('ingest.revoke.aria', { name: 'github-main' }) }));
    expect(await within(main).findByText(arText('ingest.revoke.question', { name: 'github-main' }))).toBeInTheDocument();
    expect(view.state.writes).toEqual([]);

    await view.user.click(await within(main).findByLabelText(Ar['factor.method.password']));
    await view.user.paste(PASSWORD);
    await view.user.click(within(main).getByRole('button', { name: Ar['ingest.revoke.confirm'] }));

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/ingest-tokens/${TOKEN_ID}/revoke`,
      Body: { CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await screen.findByText(arText('ingest.revoked', { name: 'github-main' }))).toBeInTheDocument();
    await waitFor(() => {
      expect(within(main).queryByRole('button', { name: arText('ingest.revoke.aria', { name: 'github-main' }) })).not.toBeInTheDocument();
    });
    expect(within(main).queryByText(arText('ingest.revoke.question', { name: 'github-main' }))).not.toBeInTheDocument();
  });

  it('can be cancelled without sending anything', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    await view.user.click(await within(main).findByRole('button', { name: arText('ingest.revoke.aria', { name: 'github-main' }) }));
    await view.user.click(await within(main).findByRole('button', { name: Ar['mfa.cancel'] }));
    expect(within(main).queryByText(arText('ingest.revoke.question', { name: 'github-main' }))).not.toBeInTheDocument();
    expect(view.state.writes).toEqual([]);
    expect(OTHER_TOKEN_ID).not.toBe(TOKEN_ID);
  });
});

describe('who sees what', () => {
  it.each([
    ['owner', true],
    ['admin', true],
    ['maintainer', true],
    ['developer', false],
  ] as const)('shows the tokens and the new button to a %s: %s', async (role, shown) => {
    mount(PAGE_URL, role);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByRole('list', { name: Ar['ingest.files.title'] })).toBeInTheDocument();
    expect(within(main).queryAllByRole('button', { name: Ar['ingest.tokens.new'] }).length > 0).toBe(shown);
    expect(within(main).queryByRole('list', { name: Ar['ingest.tokens.title'] }) !== null).toBe(shown);
    expect(within(main).queryByText(Ar['ingest.readOnly']) !== null).toBe(!shown);
    expect(within(main).queryByRole('region', { name: Ar['ingest.send.group'] }) !== null).toBe(shown);
  });

  it('shows a developer the files only and never asks the server for the tokens', async () => {
    const view = mount(PAGE_URL, 'developer');
    const main = await findPage(PROJECT_NAME);
    await within(main).findByRole('list', { name: Ar['ingest.files.title'] });
    expect(view.calls.some((call) => call.url.includes('/ingest-tokens'))).toBe(false);
  });

  it('shows a viewer nothing but the reason, and no tab', async () => {
    const view = mount(PAGE_URL, 'viewer');
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(Ar['ingest.forbidden'])).toBeInTheDocument();
    expect(within(main).queryByRole('list', { name: Ar['ingest.files.title'] })).not.toBeInTheDocument();
    expect(within(main).queryByRole('link', { name: Ar['projects.nav.ingest'] })).not.toBeInTheDocument();
    expect(view.calls.some((call) => call.url.includes('/ingested-artifacts'))).toBe(false);
  });

  it('offers no new button on an archived project', async () => {
    mount(PAGE_URL, 'owner', newIngestState(), { State: 'archived' });
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(Ar['projects.archived.blocked'])).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['ingest.tokens.new'] })).not.toBeInTheDocument();
  });
});

describe('reaching the page', () => {
  it('links to it from the project tabs', async () => {
    const view = mount(`/projects/${PROJECT_ID}/environments`);
    const main = await findPage(PROJECT_NAME);
    const tab = await within(main).findByRole('link', { name: Ar['projects.nav.ingest'] });
    expect(tab).toHaveAttribute('href', PAGE_URL);
    await view.user.click(tab);
    expect(await within(await findPage(PROJECT_NAME)).findByRole('list', { name: Ar['ingest.files.title'] })).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe(PAGE_URL);
  });

  it('links to it from the project settings', async () => {
    const view = mount(`/projects/${PROJECT_ID}/settings`);
    const main = await findPage(PROJECT_NAME);
    const link = await within(main).findByRole('link', { name: new RegExp(Ar['ingest.settings.link']) });
    expect(link).toHaveAttribute('href', PAGE_URL);
    await view.user.click(link);
    expect(await within(await findPage(PROJECT_NAME)).findByRole('list', { name: Ar['ingest.files.title'] })).toBeInTheDocument();
  });

  it('keeps the projects group current in the sidebar', async () => {
    mount();
    await findPage(PROJECT_NAME);
    const links = await screen.findAllByRole('link', { name: Ar['shell.nav.projects'], hidden: true });
    expect(links.some((link) => link.getAttribute('aria-current') === 'page')).toBe(true);
  });
});

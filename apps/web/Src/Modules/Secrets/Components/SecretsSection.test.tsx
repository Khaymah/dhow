import { screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import {
  DEPLOY_PROJECT_ID,
  newSecretsState,
  secretFixture,
  secretsFailure,
  secretsRoute,
  SECRET_VALUE,
  SECOND_PROJECT_ID,
} from '@/Core/Testing/SecretsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import type { ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';
import { SecretsSection } from '@/Modules/Secrets/Components/SecretsSection';

const ORGANIZATION: ISecretScope = { OrganizationId: ORG_ID, ProjectId: null };
const PROJECT: ISecretScope = { OrganizationId: ORG_ID, ProjectId: DEPLOY_PROJECT_ID };

const mount = (scope: ISecretScope, writable: boolean, state = newSecretsState()) => {
  startBackend({ routes: [secretsRoute(state)] });
  const onNew = vi.fn();
  const onEdit = vi.fn();
  const view = renderWithProviders(<SecretsSection scope={scope} writable={writable} onNew={onNew} onEdit={onEdit} />);
  return { ...view, state, onNew, onEdit };
};

describe('the secrets list of the organization', () => {
  it('shows the name, the description, the projects, the last change and the last use, and never a value', async () => {
    const state = newSecretsState();
    state.secrets.organization = [
      secretFixture({ LastUsedAt: new Date(Date.now() - 3_600_000).toISOString() }),
      secretFixture({
        Name: 'PUBLISH_KEY',
        Description: '',
        Visibility: 'selected_projects',
        ProjectIds: [DEPLOY_PROJECT_ID, SECOND_PROJECT_ID],
        AvailableToPullRequests: true,
      }),
    ];
    mount(ORGANIZATION, true, state);
    const list = await screen.findByRole('list', { name: Ar['secrets.list'] });
    const [first, second] = within(list).getAllByRole('listitem');
    if (first === undefined || second === undefined) throw new Error('missing rows');
    expect(within(first).getByText('DEPLOY_TOKEN')).toBeInTheDocument();
    expect(within(first).getByText('Deploy token for the API')).toBeInTheDocument();
    expect(within(first).getByText(Ar['secrets.projects.all'])).toBeInTheDocument();
    expect(within(first).queryByText(Ar['secrets.neverUsed'])).not.toBeInTheDocument();
    expect(within(second).getByText(arText('secrets.projects.selected', { count: 2 }))).toBeInTheDocument();
    expect(within(second).getByText(Ar['secrets.pullRequests'])).toBeInTheDocument();
    expect(within(second).getByText(Ar['secrets.neverUsed'])).toBeInTheDocument();
    expect(document.body.textContent).not.toContain(SECRET_VALUE);
  });

  it('offers adding and changing only when the caller may write', async () => {
    const state = newSecretsState();
    state.secrets.organization = [secretFixture()];
    const view = mount(ORGANIZATION, true, state);
    await view.user.click(await screen.findByRole('button', { name: arText('secrets.change.aria', { name: 'DEPLOY_TOKEN' }) }));
    expect(view.onEdit).toHaveBeenCalledWith('DEPLOY_TOKEN');
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.add'] }));
    expect(view.onNew).toHaveBeenCalledOnce();
  });

  it('shows the names and no write button to a caller who cannot write', async () => {
    const state = newSecretsState();
    state.secrets.organization = [secretFixture()];
    mount(ORGANIZATION, false, state);
    expect(await screen.findByText('DEPLOY_TOKEN')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('says so when there are no secrets, with the add button for a writer', async () => {
    const view = mount(ORGANIZATION, true);
    expect(await screen.findByText(Ar['secrets.empty'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.add'] }));
    expect(view.onNew).toHaveBeenCalledOnce();
  });

  it('shows the server message when loading fails and tries again on request', async () => {
    const state = newSecretsState();
    state.failLists = secretsFailure('INTERNAL', 'تعذر الوصول إلى قاعدة البيانات.', 500);
    const view = mount(ORGANIZATION, true, state);
    expect(await screen.findByText(Ar['secrets.error'])).toBeInTheDocument();
    expect(screen.getByText('تعذر الوصول إلى قاعدة البيانات.')).toBeInTheDocument();
    state.failLists = null;
    state.secrets.organization = [secretFixture()];
    await view.user.click(screen.getByRole('button', { name: Ar['status.retry'] }));
    expect(await screen.findByText('DEPLOY_TOKEN')).toBeInTheDocument();
  });
});

describe('the secrets list of a project', () => {
  it('marks a project secret that covers an organization secret', async () => {
    const state = newSecretsState();
    state.secrets.project = [
      secretFixture({ Scope: 'project', Overrides: ['organization'] }),
      secretFixture({ Name: 'LOCAL_ONLY', Scope: 'project' }),
    ];
    mount(PROJECT, false, state);
    const list = await screen.findByRole('list', { name: Ar['secrets.list'] });
    const [covering, plain] = within(list).getAllByRole('listitem');
    if (covering === undefined || plain === undefined) throw new Error('missing rows');
    expect(within(covering).getByText(Ar['secrets.overrides.organization'])).toBeInTheDocument();
    expect(within(plain).queryByText(Ar['secrets.overrides.organization'])).not.toBeInTheDocument();
    expect(within(covering).queryByText(Ar['secrets.projects'])).not.toBeInTheDocument();
  });
});

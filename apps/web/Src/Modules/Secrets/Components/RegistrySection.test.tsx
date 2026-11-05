import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import {
  CREDENTIAL_ID,
  credentialFixture,
  DEPLOY_PROJECT_ID,
  newSecretsState,
  REGISTRY_PASSWORD,
  secretsFailure,
  secretsRoute,
  browserTraces,
} from '@/Core/Testing/SecretsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import type { ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';
import { RegistryFormBody } from '@/Modules/Secrets/Components/RegistryFormBody';
import { RegistrySection } from '@/Modules/Secrets/Components/RegistrySection';

const ORGANIZATION: ISecretScope = { OrganizationId: ORG_ID, ProjectId: null };

const mountList = (writable: boolean, state = newSecretsState()) => {
  startBackend({ routes: [secretsRoute(state)] });
  const onNew = vi.fn();
  const onEdit = vi.fn();
  const view = renderWithProviders(<RegistrySection scope={ORGANIZATION} writable={writable} onNew={onNew} onEdit={onEdit} />);
  return { ...view, state, onNew, onEdit };
};

const mountForm = (props: { projectId?: string; credentialId?: string }, role = 'owner', state = newSecretsState()) => {
  startBackend({ role, routes: [secretsRoute(state)] });
  const onDone = vi.fn();
  const view = renderWithProviders(<RegistryFormBody {...props} onDone={onDone} />);
  return { ...view, state, onDone };
};

const passwordField = () => screen.getByLabelText(Ar['registry.form.password']);

const leaked = (view: ReturnType<typeof mountForm>): string => browserTraces(view.client);

describe('the registry accounts list', () => {
  it('shows the host and the username and never a password', async () => {
    const state = newSecretsState();
    state.credentials.organization = [credentialFixture()];
    mountList(true, state);
    const list = await screen.findByRole('list', { name: Ar['registry.list'] });
    expect(within(list).getByText('ghcr.io')).toBeInTheDocument();
    expect(within(list).getByText('deploy-bot')).toBeInTheDocument();
    expect(document.body.textContent).not.toContain('Password');
  });

  it('has no write button for a caller who cannot write', async () => {
    const state = newSecretsState();
    state.credentials.organization = [credentialFixture()];
    mountList(false, state);
    expect(await screen.findByText('ghcr.io')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('opens the change slide, and removes an account only after asking', async () => {
    const state = newSecretsState();
    state.credentials.organization = [credentialFixture()];
    const view = mountList(true, state);
    await view.user.click(await screen.findByRole('button', { name: arText('registry.change.aria', { host: 'ghcr.io' }) }));
    expect(view.onEdit).toHaveBeenCalledWith(CREDENTIAL_ID);
    await view.user.click(screen.getByRole('button', { name: arText('registry.remove.aria', { host: 'ghcr.io' }) }));
    expect(view.state.writes).toEqual([]);
    await view.user.click(screen.getByRole('button', { name: Ar['registry.remove.confirm'] }));
    await waitFor(() => {
      expect(view.state.writes).toEqual([
        { Method: 'POST', Path: `/organizations/${ORG_ID}/registry-credentials/${CREDENTIAL_ID}/remove`, Body: null },
      ]);
    });
    expect(await screen.findByText(Ar['registry.empty'])).toBeInTheDocument();
  });
});

describe('writing a registry account', () => {
  it('creates an account, clears the password, and keeps it out of the page and the browser', async () => {
    const view = mountForm({});
    await view.user.type(await screen.findByLabelText(Ar['registry.form.host']), 'ghcr.io');
    await view.user.type(screen.getByLabelText(Ar['registry.form.username']), 'deploy-bot');
    await view.user.type(passwordField(), REGISTRY_PASSWORD);
    await view.user.click(screen.getByRole('button', { name: Ar['registry.form.submit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes).toEqual([
      {
        Method: 'POST',
        Path: `/organizations/${ORG_ID}/registry-credentials`,
        Body: { Host: 'ghcr.io', Username: 'deploy-bot', Password: REGISTRY_PASSWORD },
      },
    ]);
    expect(passwordField()).toHaveValue('');
    expect(leaked(view)).not.toContain(REGISTRY_PASSWORD);
  });

  it('changes the username and password of an account whose host stays fixed', async () => {
    const state = newSecretsState();
    state.credentials.project = [credentialFixture({ Scope: 'project' })];
    const view = mountForm({ projectId: DEPLOY_PROJECT_ID, credentialId: CREDENTIAL_ID }, 'maintainer', state);
    expect(await screen.findByText('ghcr.io')).toBeInTheDocument();
    expect(screen.queryByLabelText(Ar['registry.form.host'])).not.toBeInTheDocument();
    expect(screen.getByLabelText(Ar['registry.form.username'])).toHaveValue('deploy-bot');
    expect(passwordField()).toHaveValue('');
    await view.user.clear(screen.getByLabelText(Ar['registry.form.username']));
    await view.user.type(screen.getByLabelText(Ar['registry.form.username']), 'new-bot');
    await view.user.type(passwordField(), REGISTRY_PASSWORD);
    await view.user.click(screen.getByRole('button', { name: Ar['registry.form.submit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes).toEqual([
      {
        Method: 'PUT',
        Path: `/organizations/${ORG_ID}/projects/${DEPLOY_PROJECT_ID}/registry-credentials/${CREDENTIAL_ID}`,
        Body: { Username: 'new-bot', Password: REGISTRY_PASSWORD },
      },
    ]);
    expect(passwordField()).toHaveValue('');
    expect(leaked(view)).not.toContain(REGISTRY_PASSWORD);
  });

  it('asks for the host, the username and the password before sending anything', async () => {
    const view = mountForm({});
    await view.user.click(await screen.findByRole('button', { name: Ar['registry.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['registry.form.error.host']);
    await view.user.type(screen.getByLabelText(Ar['registry.form.host']), 'ghcr.io');
    await view.user.click(screen.getByRole('button', { name: Ar['registry.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['registry.form.error.username']);
    await view.user.type(screen.getByLabelText(Ar['registry.form.username']), 'bot');
    await view.user.click(screen.getByRole('button', { name: Ar['registry.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['registry.form.error.password']);
    expect(view.state.writes).toEqual([]);
  });

  it('shows the server message when the host already has an account, and keeps the password for another try', async () => {
    const state = newSecretsState();
    state.failWrites = secretsFailure('REGISTRY_CREDENTIAL_EXISTS', 'يوجد حساب لهذا السجل بالفعل.', 409);
    const view = mountForm({}, 'owner', state);
    await view.user.type(await screen.findByLabelText(Ar['registry.form.host']), 'ghcr.io');
    await view.user.type(screen.getByLabelText(Ar['registry.form.username']), 'bot');
    await view.user.type(passwordField(), REGISTRY_PASSWORD);
    await view.user.click(screen.getByRole('button', { name: Ar['registry.form.submit'] }));
    expect(await screen.findByText('يوجد حساب لهذا السجل بالفعل.')).toBeInTheDocument();
    expect(view.onDone).not.toHaveBeenCalled();
  });

  it('keeps a developer out of the form', async () => {
    mountForm({}, 'developer');
    expect(await screen.findByText(Ar['secrets.form.forbidden'])).toBeInTheDocument();
  });
});

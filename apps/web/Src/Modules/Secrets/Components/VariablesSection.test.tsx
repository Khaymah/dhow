import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { DEPLOY_PROJECT_ID, newSecretsState, secretsFailure, secretsRoute, variableFixture } from '@/Core/Testing/SecretsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import type { ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';
import { VariableFormBody } from '@/Modules/Secrets/Components/VariableFormBody';
import { VariablesSection } from '@/Modules/Secrets/Components/VariablesSection';

const ORGANIZATION: ISecretScope = { OrganizationId: ORG_ID, ProjectId: null };
const PROJECT: ISecretScope = { OrganizationId: ORG_ID, ProjectId: DEPLOY_PROJECT_ID };

const mountList = (scope: ISecretScope, writable: boolean, state = newSecretsState()) => {
  startBackend({ routes: [secretsRoute(state)] });
  const onNew = vi.fn();
  const onEdit = vi.fn();
  const view = renderWithProviders(<VariablesSection scope={scope} writable={writable} onNew={onNew} onEdit={onEdit} />);
  return { ...view, state, onNew, onEdit };
};

const mountForm = (props: { projectId?: string; name?: string }, role = 'owner', state = newSecretsState()) => {
  startBackend({ role, routes: [secretsRoute(state)] });
  const onDone = vi.fn();
  const view = renderWithProviders(<VariableFormBody {...props} onDone={onDone} />);
  return { ...view, state, onDone };
};

describe('the variables list', () => {
  it('shows each name with its value, and the mark for a variable that covers the organization one', async () => {
    const state = newSecretsState();
    state.variables.project = [
      variableFixture({ Scope: 'project', Overrides: ['organization'] }),
      variableFixture({ Name: 'REGION', Value: 'eu-west' }),
    ];
    mountList(PROJECT, false, state);
    const list = await screen.findByRole('list', { name: Ar['variables.list'] });
    const [first, second] = within(list).getAllByRole('listitem');
    if (first === undefined || second === undefined) throw new Error('missing rows');
    expect(within(first).getByText('app.example.com')).toBeInTheDocument();
    expect(within(first).getByText(Ar['variables.overrides.organization'])).toBeInTheDocument();
    expect(within(second).getByText('eu-west')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('opens the edit slide and asks before removing', async () => {
    const state = newSecretsState();
    state.variables.organization = [variableFixture()];
    const view = mountList(ORGANIZATION, true, state);
    await view.user.click(await screen.findByRole('button', { name: arText('variables.edit.aria', { name: 'DEPLOY_HOST' }) }));
    expect(view.onEdit).toHaveBeenCalledWith('DEPLOY_HOST');

    await view.user.click(screen.getByRole('button', { name: arText('variables.remove.aria', { name: 'DEPLOY_HOST' }) }));
    expect(view.state.writes).toEqual([]);
    expect(screen.getByText(arText('variables.remove.question', { name: 'DEPLOY_HOST' }))).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['variables.remove.confirm'] }));
    await waitFor(() => {
      expect(view.state.writes).toEqual([{ Method: 'POST', Path: `/organizations/${ORG_ID}/variables/DEPLOY_HOST/remove`, Body: null }]);
    });
    expect(await screen.findByText(Ar['variables.empty'])).toBeInTheDocument();
  });

  it('shows the server message when removing fails', async () => {
    const state = newSecretsState();
    state.variables.organization = [variableFixture()];
    state.failWrites = secretsFailure('FORBIDDEN', 'ليست لديك صلاحية حذف المتغيّرات.', 403);
    const view = mountList(ORGANIZATION, true, state);
    await view.user.click(await screen.findByRole('button', { name: arText('variables.remove.aria', { name: 'DEPLOY_HOST' }) }));
    await view.user.click(screen.getByRole('button', { name: Ar['variables.remove.confirm'] }));
    expect(await screen.findByText('ليست لديك صلاحية حذف المتغيّرات.')).toBeInTheDocument();
  });
});

describe('writing a variable', () => {
  it('creates a variable in the organization', async () => {
    const view = mountForm({});
    await view.user.type(await screen.findByLabelText(Ar['variables.form.name']), 'REGION');
    await view.user.type(screen.getByLabelText(Ar['variables.form.value']), 'eu-west');
    await view.user.click(screen.getByRole('button', { name: Ar['variables.form.submit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes).toEqual([{ Method: 'PUT', Path: `/organizations/${ORG_ID}/variables/REGION`, Body: { Value: 'eu-west' } }]);
  });

  it('edits a variable with its current value in the field', async () => {
    const state = newSecretsState();
    state.variables.project = [variableFixture({ Scope: 'project' })];
    const view = mountForm({ projectId: DEPLOY_PROJECT_ID, name: 'DEPLOY_HOST' }, 'maintainer', state);
    const field = await screen.findByLabelText(Ar['variables.form.value']);
    expect(field).toHaveValue('app.example.com');
    await view.user.clear(field);
    await view.user.type(field, 'new.example.com');
    await view.user.click(screen.getByRole('button', { name: Ar['variables.form.submit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes).toEqual([
      {
        Method: 'PUT',
        Path: `/organizations/${ORG_ID}/projects/${DEPLOY_PROJECT_ID}/variables/DEPLOY_HOST`,
        Body: { Value: 'new.example.com' },
      },
    ]);
  });

  it('refuses a bad name and a value over 4 KiB before asking the server', async () => {
    const view = mountForm({});
    await view.user.type(await screen.findByLabelText(Ar['variables.form.name']), '9bad');
    await view.user.click(screen.getByRole('button', { name: Ar['variables.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['secrets.form.error.name']);
    await view.user.clear(screen.getByLabelText(Ar['variables.form.name']));
    await view.user.type(screen.getByLabelText(Ar['variables.form.name']), 'BIG');
    await view.user.click(screen.getByLabelText(Ar['variables.form.value']));
    await view.user.paste('x'.repeat(4097));
    await view.user.click(screen.getByRole('button', { name: Ar['variables.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['variables.form.error.large']);
    expect(view.state.writes).toEqual([]);
  });

  it('shows the server message on failure', async () => {
    const state = newSecretsState();
    state.failWrites = secretsFailure('VARIABLE_LIMIT_REACHED', 'وصلت إلى الحد الأقصى لعدد المتغيّرات.', 409);
    const view = mountForm({}, 'owner', state);
    await view.user.type(await screen.findByLabelText(Ar['variables.form.name']), 'REGION');
    await view.user.click(screen.getByRole('button', { name: Ar['variables.form.submit'] }));
    expect(await screen.findByText('وصلت إلى الحد الأقصى لعدد المتغيّرات.')).toBeInTheDocument();
    expect(view.onDone).not.toHaveBeenCalled();
  });

  it('keeps a maintainer out of the organization form', async () => {
    mountForm({}, 'maintainer');
    expect(await screen.findByText(Ar['secrets.form.forbidden'])).toBeInTheDocument();
  });
});

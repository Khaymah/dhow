import { screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import {
  DEPLOY_PROJECT_ID,
  newSecretsState,
  secretFixture,
  secretsFailure,
  secretsRoute,
  SECRET_VALUE,
  browserTraces,
  SECOND_PROJECT_ID,
} from '@/Core/Testing/SecretsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { SecretFormBody, type ISecretFormBodyProps } from '@/Modules/Secrets/Components/SecretFormBody';

const projects = [
  projectFixture({ Id: DEPLOY_PROJECT_ID, Name: 'Web app', Slug: 'web-app' }),
  projectFixture({ Id: SECOND_PROJECT_ID, Name: 'Billing', Slug: 'billing' }),
];

const mount = (props: Omit<ISecretFormBodyProps, 'onDone'> = {}, role = 'owner', state = newSecretsState()) => {
  startBackend({ role, routes: [secretsRoute(state), projectsRoute({ projects })] });
  const onDone = vi.fn();
  const view = renderWithProviders(<SecretFormBody {...props} onDone={onDone} />);
  return { ...view, state, onDone };
};

const valueField = async (): Promise<HTMLTextAreaElement> => {
  const field = await screen.findByLabelText(new RegExp(`^(${Ar['secrets.form.value']}|${Ar['secrets.form.value.edit']})$`));
  if (!(field instanceof HTMLTextAreaElement)) throw new Error('the value field is not a textarea');
  return field;
};

const fillNew = async (view: ReturnType<typeof mount>, name: string, value: string) => {
  await view.user.type(await screen.findByLabelText(Ar['secrets.form.name']), name);
  await view.user.click(await valueField());
  await view.user.paste(value);
};

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

describe('writing a new secret', () => {
  it('sends the value once, clears the field, and keeps the value out of the page, the address and the browser storage', async () => {
    const view = mount();
    await fillNew(view, 'API_KEY', SECRET_VALUE);
    await view.user.type(screen.getByLabelText(Ar['secrets.form.description']), 'Key for the API');
    await view.user.click(screen.getByRole('checkbox', { name: Ar['secrets.form.pullRequests'] }));
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));

    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes).toEqual([
      {
        Method: 'PUT',
        Path: `/organizations/${ORG_ID}/secrets/API_KEY`,
        Body: {
          Value: SECRET_VALUE,
          Description: 'Key for the API',
          AvailableToPullRequests: true,
          Visibility: 'all_projects',
          ProjectIds: [],
        },
      },
    ]);

    expect((await valueField()).value).toBe('');
    const everything = `${browserTraces(view.client)}\n${document.body.textContent}`;
    expect(everything).not.toContain(SECRET_VALUE);
  });

  it('does not send a name that is reserved, malformed, or a value that is too short', async () => {
    const view = mount();
    await fillNew(view, 'DHOW_TOKEN', SECRET_VALUE);
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['secrets.form.error.reserved']);

    await view.user.clear(screen.getByLabelText(Ar['secrets.form.name']));
    await view.user.type(screen.getByLabelText(Ar['secrets.form.name']), '1BAD');
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['secrets.form.error.name']);

    await view.user.clear(screen.getByLabelText(Ar['secrets.form.name']));
    await view.user.type(screen.getByLabelText(Ar['secrets.form.name']), 'GOOD_NAME');
    await view.user.clear(await valueField());
    await view.user.type(await valueField(), 'short');
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['secrets.form.error.short']);
    expect(view.state.writes).toEqual([]);
  });

  it('shows the message the server sends and keeps the form so the caller can try again', async () => {
    const state = newSecretsState();
    state.failWrites = secretsFailure('SECRET_LIMIT_REACHED', 'وصلت إلى الحد الأقصى لعدد الأسرار.', 409);
    const view = mount({}, 'owner', state);
    await fillNew(view, 'API_KEY', SECRET_VALUE);
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    expect(await screen.findByText('وصلت إلى الحد الأقصى لعدد الأسرار.')).toBeInTheDocument();
    expect(view.onDone).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: Ar['secrets.form.submit'] })).toBeEnabled();
  });

  it('writes a project secret to the project endpoint, without a choice of projects', async () => {
    const view = mount({ projectId: DEPLOY_PROJECT_ID }, 'maintainer');
    await fillNew(view, 'API_KEY', SECRET_VALUE);
    expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    const [write] = view.state.writes;
    expect(write?.Path).toBe(`/organizations/${ORG_ID}/projects/${DEPLOY_PROJECT_ID}/secrets/API_KEY`);
    expect(write?.Body).toEqual({ Value: SECRET_VALUE, Description: '', AvailableToPullRequests: false });
  });

  it('does not let a maintainer write an organization secret', async () => {
    mount({}, 'maintainer');
    expect(await screen.findByText(Ar['secrets.form.forbidden'])).toBeInTheDocument();
    expect(screen.queryByLabelText(Ar['secrets.form.name'])).not.toBeInTheDocument();
  });
});

describe('choosing the projects of an organization secret', () => {
  it('sends the chosen projects when the secret is limited to them', async () => {
    const view = mount();
    await fillNew(view, 'API_KEY', SECRET_VALUE);
    await view.user.click(screen.getByRole('radio', { name: new RegExp(Ar['secrets.form.visibility.selected']) }));
    await view.user.click(await screen.findByRole('checkbox', { name: /Billing/ }));
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes[0]?.Body).toMatchObject({ Visibility: 'selected_projects', ProjectIds: [SECOND_PROJECT_ID] });
  });

  it('asks for at least one project when the secret is limited to selected projects', async () => {
    const view = mount();
    await fillNew(view, 'API_KEY', SECRET_VALUE);
    await view.user.click(screen.getByRole('radio', { name: new RegExp(Ar['secrets.form.visibility.selected']) }));
    await screen.findByRole('checkbox', { name: /Web app/ });
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent(Ar['secrets.form.error.projects']);
    expect(view.state.writes).toEqual([]);
  });

  it('sends no projects when the caller goes back to all projects', async () => {
    const view = mount();
    await fillNew(view, 'API_KEY', SECRET_VALUE);
    await view.user.click(screen.getByRole('radio', { name: new RegExp(Ar['secrets.form.visibility.selected']) }));
    await view.user.click(await screen.findByRole('checkbox', { name: /Web app/ }));
    await view.user.click(screen.getByRole('radio', { name: new RegExp(Ar['secrets.form.visibility.all']) }));
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes[0]?.Body).toMatchObject({ Visibility: 'all_projects', ProjectIds: [] });
  });
});

describe('changing the value of a secret', () => {
  const existing = () => {
    const state = newSecretsState();
    state.secrets.organization = [secretFixture({ Visibility: 'selected_projects', ProjectIds: [DEPLOY_PROJECT_ID] })];
    return state;
  };

  it('writes a new version with the settings it already has and clears the field', async () => {
    const view = mount({ name: 'DEPLOY_TOKEN' }, 'admin', existing());
    const field = await valueField();
    expect(field.value).toBe('');
    expect(screen.getByLabelText(Ar['secrets.form.description'])).toHaveValue('Deploy token for the API');
    await view.user.click(field);
    await view.user.paste(SECRET_VALUE);
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit.edit'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes).toEqual([
      {
        Method: 'PUT',
        Path: `/organizations/${ORG_ID}/secrets/DEPLOY_TOKEN`,
        Body: {
          Value: SECRET_VALUE,
          Description: 'Deploy token for the API',
          AvailableToPullRequests: false,
          Visibility: 'selected_projects',
          ProjectIds: [DEPLOY_PROJECT_ID],
        },
      },
    ]);
    expect(view.state.secrets.organization?.[0]?.Version).toBe(3);
    expect((await valueField()).value).toBe('');
    expect(document.documentElement.outerHTML).not.toContain(SECRET_VALUE);
  });

  it('says so when the secret is gone', async () => {
    mount({ name: 'MISSING_ONE' }, 'admin', existing());
    expect(await screen.findByText(Ar['secrets.form.notFound'])).toBeInTheDocument();
  });

  it('removes the secret only after the caller confirms', async () => {
    const view = mount({ name: 'DEPLOY_TOKEN' }, 'admin', existing());
    const zone = await screen.findByRole('region', { name: Ar['secrets.danger.title'] });
    await view.user.click(within(zone).getByRole('button', { name: Ar['secrets.remove'] }));
    expect(view.state.writes).toEqual([]);
    expect(within(zone).getByText(arText('secrets.remove.question', { name: 'DEPLOY_TOKEN' }))).toBeInTheDocument();
    await view.user.click(within(zone).getByRole('button', { name: Ar['mfa.cancel'] }));
    expect(view.state.writes).toEqual([]);
    await view.user.click(within(zone).getByRole('button', { name: Ar['secrets.remove'] }));
    await view.user.click(within(zone).getByRole('button', { name: Ar['secrets.remove.confirm'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.state.writes).toEqual([{ Method: 'POST', Path: `/organizations/${ORG_ID}/secrets/DEPLOY_TOKEN/remove`, Body: null }]);
    expect(view.state.secrets.organization).toEqual([]);
  });

  it('shows the server message when removing fails', async () => {
    const state = existing();
    state.failWrites = secretsFailure('FORBIDDEN', 'هذا الإجراء يحتاج إلى خطوة تحقق ثانية.', 403);
    const view = mount({ name: 'DEPLOY_TOKEN' }, 'admin', state);
    const zone = await screen.findByRole('region', { name: Ar['secrets.danger.title'] });
    await view.user.click(within(zone).getByRole('button', { name: Ar['secrets.remove'] }));
    await view.user.click(within(zone).getByRole('button', { name: Ar['secrets.remove.confirm'] }));
    expect(await within(zone).findByText('هذا الإجراء يحتاج إلى خطوة تحقق ثانية.')).toBeInTheDocument();
    expect(view.onDone).not.toHaveBeenCalled();
  });
});

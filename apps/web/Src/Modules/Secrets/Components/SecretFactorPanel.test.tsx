import { screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { backendFailure, startBackend, type TRouteHandler } from '@/Core/Testing/Backend';
import { ok, PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { GOOD_PASSWORD } from '@/Core/Testing/MfaBackend';
import { secretFixture, SECRET_VALUE } from '@/Core/Testing/SecretsBackend';
import { Ar } from '@/I18n/Locales/Ar';
import { SecretFormBody } from '@/Modules/Secrets/Components/SecretFormBody';

const TARGET_KEY = 'PROD_DEPLOY_KEY';
const ENVIRONMENT = 'production';

interface IWrite {
  Body: { Value?: string; CurrentFactor?: { Method: string; Password?: string } };
}

const mount = (needsFactor: boolean) => {
  const writes: IWrite[] = [];
  const answer = (body: IWrite['Body']): Response => {
    writes.push({ Body: body });
    if (needsFactor && body.CurrentFactor === undefined) return backendFailure('TARGET_KEY_NEEDS_FACTOR', Ar['secrets.factor.question'], 403);
    if (body.CurrentFactor !== undefined && body.CurrentFactor.Password !== GOOD_PASSWORD) return backendFailure('INVALID_CURRENT_FACTOR', 'no', 403);
    return ok(secretFixture({ Name: TARGET_KEY, Scope: 'environment', Version: 2 }));
  };
  const route: TRouteHandler = (request, url) => {
    const base = `/environments/${ENVIRONMENT}/secrets`;
    if (request.method === 'GET' && url.pathname.endsWith(base)) return ok([secretFixture({ Name: TARGET_KEY, Scope: 'environment' })]);
    if (request.method === 'PUT' && url.pathname.endsWith(`${base}/${TARGET_KEY}`)) return request.json().then((body) => answer(body as IWrite['Body']));
    return undefined;
  };
  startBackend({ role: 'owner', routes: [route] });
  const onDone = vi.fn();
  const view = renderWithProviders(<SecretFormBody projectId={PROJECT_ID} environmentName={ENVIRONMENT} name={TARGET_KEY} onDone={onDone} />);
  return { ...view, writes, onDone };
};

const typeValue = async (view: ReturnType<typeof mount>) => {
  await view.user.click(await screen.findByLabelText(Ar['secrets.form.value.edit']));
  await view.user.paste(SECRET_VALUE);
  await view.user.click(screen.getByRole('button', { name: Ar['secrets.form.submit.edit'] }));
};

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

describe('overwriting the secret that is the key of a deploy target', () => {
  it('asks for the second step after the server says so, then sends the value again with it', async () => {
    const view = mount(true);
    await typeValue(view);

    const password = await screen.findByLabelText(Ar['factor.method.password']);
    expect(screen.getByText(Ar['secrets.factor.question'])).toBeInTheDocument();
    expect(view.onDone).not.toHaveBeenCalled();
    expect(view.writes).toHaveLength(1);
    expect(view.writes[0]?.Body.CurrentFactor).toBeUndefined();

    await view.user.type(password, GOOD_PASSWORD);
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.factor.confirm'] }));
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(view.writes).toHaveLength(2);
    expect(view.writes[1]?.Body).toMatchObject({ Value: SECRET_VALUE, CurrentFactor: { Method: 'password', Password: GOOD_PASSWORD } });
  });

  it('keeps the panel and says why when the proof is wrong', async () => {
    const view = mount(true);
    await typeValue(view);
    await view.user.type(await screen.findByLabelText(Ar['factor.method.password']), 'not the password');
    await view.user.click(screen.getByRole('button', { name: Ar['secrets.factor.confirm'] }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(view.onDone).not.toHaveBeenCalled();
    expect(screen.getByText(Ar['secrets.factor.question'])).toBeInTheDocument();
  });

  it('closes the panel on cancel and sends nothing more', async () => {
    const view = mount(true);
    await typeValue(view);
    await screen.findByLabelText(Ar['factor.method.password']);
    await view.user.click(within(document.body).getByRole('button', { name: Ar['mfa.cancel'] }));
    await waitFor(() => {
      expect(screen.queryByText(Ar['secrets.factor.question'])).not.toBeInTheDocument();
    });
    expect(view.writes).toHaveLength(1);
    expect(view.onDone).not.toHaveBeenCalled();
  });

  it('asks nothing for a secret that is not a key', async () => {
    const view = mount(false);
    await typeValue(view);
    await waitFor(() => {
      expect(view.onDone).toHaveBeenCalledOnce();
    });
    expect(screen.queryByText(Ar['secrets.factor.question'])).not.toBeInTheDocument();
    expect(view.writes).toHaveLength(1);
  });
});

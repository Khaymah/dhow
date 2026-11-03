import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, stubFetch } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { LoginPage } from '@/Modules/Session/Pages/LoginPage';
import { isolate } from '@/I18n/Translate';

const LOGIN = { User: { Id: 'u', Email: 'sabry@future.dev', Name: 'Sabry', Locale: 'ar' }, CsrfToken: 'csrf', ExpiresAt: '2026-11-10T08:00:00Z' };

const pending = (methods: string[]) =>
  envelope({ Success: true, Message: 'session.mfa.required', Data: { Methods: methods, ExpiresAt: '2026-10-27T08:05:00Z' } }, 202);

const mount = async (methods: string[], second: (request: Request) => Response) => {
  const { calls } = stubFetch((request) => (new URL(request.url).pathname.endsWith('/sessions/mfa') ? second(request) : pending(methods)));
  const onSignedIn = vi.fn();
  const view = renderWithProviders(<LoginPage onSignedIn={onSignedIn} />);
  await view.user.type(screen.getByLabelText(Ar['field.email']), 'sabry@future.dev');
  await view.user.type(screen.getByLabelText(Ar['field.password']), 'correct horse battery staple');
  await view.user.click(screen.getByRole('button', { name: Ar['login.submit'] }));
  await screen.findByRole('heading', { name: Ar['mfa.step.title'] });
  return { ...view, calls, onSignedIn };
};

describe('the second sign-in step', () => {
  it('asks for the authenticator code after a correct password and signs in with it', async () => {
    const { user, calls, onSignedIn } = await mount(['totp', 'recovery_code'], () => envelope({ Success: true, Data: LOGIN }));
    expect(screen.getByText(Ar['mfa.step.lead.totp'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['mfa.step.usePasskey'] })).not.toBeInTheDocument();
    await user.type(screen.getByLabelText(Ar['mfa.step.code']), '123 456');
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.submit'] }));
    await waitFor(() => {
      expect(onSignedIn).toHaveBeenCalledOnce();
    });
    const sent = calls.at(-1);
    expect(sent?.method).toBe('POST');
    expect(await sent?.json()).toEqual({ Method: 'totp', Code: '123456' });
  });

  it('checks the code shape before calling the server', async () => {
    const { user, calls } = await mount(['totp'], () => envelope({ Success: true, Data: LOGIN }));
    await user.type(screen.getByLabelText(Ar['mfa.step.code']), '12a');
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.submit'] }));
    expect(await screen.findByText(Ar['mfa.step.error.code'])).toBeInTheDocument();
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(1);
  });

  it('switches to a recovery code and sends it as written', async () => {
    const { user, calls } = await mount(['totp', 'recovery_code'], () => envelope({ Success: true, Data: LOGIN }));
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.useRecovery'] }));
    expect(screen.getByText(Ar['mfa.step.lead.recovery'])).toBeInTheDocument();
    await user.type(screen.getByLabelText(Ar['mfa.step.recoveryCode']), '7KQ2-M9XD-P4TR-82WN');
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.submit'] }));
    await waitFor(async () => {
      expect(await calls.at(-1)?.clone().json()).toEqual({ Method: 'recovery_code', Code: '7KQ2M9XDP4TR82WN' });
    });
  });

  it('shows the generic error for a wrong code and keeps the step open', async () => {
    const { user } = await mount(['totp'], () => backendFailure('INVALID_MFA_CODE', 'الكود غير صحيح.', 401));
    await user.type(screen.getByLabelText(Ar['mfa.step.code']), '000000');
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.submit'] }));
    expect(await screen.findByText('الكود غير صحيح.')).toBeInTheDocument();
    expect(screen.getByLabelText(Ar['mfa.step.code'])).toHaveValue('');
  });

  it('goes back to the password with a notice when the step expired', async () => {
    const { user } = await mount(['totp'], () => backendFailure('MFA_CHALLENGE_EXPIRED', 'x', 401));
    await user.type(screen.getByLabelText(Ar['mfa.step.code']), '000000');
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.submit'] }));
    expect(await screen.findByText(Ar['mfa.step.expired'])).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: Ar['login.title'] })).toBeInTheDocument();
  });

  it('counts down when the server throttles the step', async () => {
    const { user } = await mount(['totp'], () => backendFailure('INVALID_MFA_CODE', 'x', 401, { 'Retry-After': '30' }));
    await user.type(screen.getByLabelText(Ar['mfa.step.code']), '000000');
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.submit'] }));
    expect(await screen.findByRole('timer')).toHaveTextContent('0:30');
    expect(screen.getByRole('button', { name: Ar['throttle.retryIn'].replace('{time}', isolate('0:30')) })).toBeDisabled();
  });

  it('returns to the password form on request', async () => {
    const { user } = await mount(['totp'], () => envelope({ Success: true, Data: LOGIN }));
    await user.click(screen.getByRole('button', { name: Ar['mfa.step.back'] }));
    expect(screen.getByRole('heading', { name: Ar['login.title'] })).toBeInTheDocument();
    expect(screen.queryByText(Ar['mfa.step.expired'])).not.toBeInTheDocument();
  });
});

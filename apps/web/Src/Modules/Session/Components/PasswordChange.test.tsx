import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { backendFailure, startBackend, type IBackendOptions } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';
import { useToastStore } from '@/Core/Store/useToastStore';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const CURRENT = 'the old password 123';
const NEXT = 'correct horse battery staple 42';

const mount = async (options: IBackendOptions = {}) => {
  const backend = startBackend(options);
  const router = createAppRouter(createMemoryHistory({ initialEntries: ['/account/security?slide=account-password'] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = await screen.findByRole('dialog', { name: Ar['password.slide.title'] });
  await within(dialog).findByLabelText(Ar['password.current']);
  return { ...backend, router, user, dialog };
};

const type = async (user: ReturnType<typeof userEvent.setup>, current: string, next: string, again: string) => {
  if (current !== '') await user.type(screen.getByLabelText(Ar['password.current']), current);
  if (next !== '') await user.type(screen.getByLabelText(Ar['password.new']), next);
  if (again !== '') await user.type(screen.getByLabelText(Ar['password.confirm']), again);
};

const submit = (user: ReturnType<typeof userEvent.setup>) =>
  user.click(screen.getByRole('button', { name: Ar['password.change.submit'] }));

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('changing the password', () => {
  it('opens the form over the security page and says other devices will be signed out', async () => {
    const { router, dialog } = await mount();
    expect(router.state.location.href).toBe('/account/security?slide=account-password');
    expect(within(dialog).getByText(Ar['password.change.notice'])).toBeInTheDocument();
    expect(within(dialog).getByRole('meter', { name: Ar['password.strength.label'] })).toBeInTheDocument();
    expect(within(dialog).getByLabelText(Ar['password.current'])).toHaveAttribute('autocomplete', 'current-password');
    expect(within(dialog).getByLabelText(Ar['password.new'])).toHaveAttribute('autocomplete', 'new-password');
  });

  it('sends the passwords with the CSRF token, signs the other devices out, clears the fields and goes back', async () => {
    const { user, calls, router } = await mount();
    await type(user, CURRENT, NEXT, NEXT);
    await submit(user);
    await waitFor(() => {
      expect(router.state.location.href).toBe('/account/security');
    });
    const put = calls.find((call) => call.method === 'PUT');
    expect(new URL(put?.url ?? '').pathname).toBe('/api/v1/me/password');
    expect(put?.headers.get('X-CSRF-Token')).toBe('csrf-token-1');
    expect(await put?.clone().json()).toEqual({ CurrentPassword: CURRENT, NewPassword: NEXT });
    const toast = useToastStore.getState().Toasts.find((item) => item.Id === 'password-changed');
    expect(toast?.Title).toBe(Ar['password.change.done']);
    expect(toast?.Detail).toBe(arText('password.change.done.others', { count: 2 }));
    expect(document.body.innerHTML).not.toContain(NEXT);
    expect(await screen.findByText(Ar['account.sessions.count.one'])).toBeInTheDocument();
  });

  it('checks the form first and moves focus to the first problem', async () => {
    const { user, calls } = await mount();
    await submit(user);
    expect(await screen.findByText(Ar['password.error.currentRequired'])).toBeInTheDocument();
    expect(screen.getByLabelText(Ar['password.current'])).toHaveFocus();
    expect(screen.getByText(Ar['password.error.tooShort'])).toBeInTheDocument();
    expect(calls.filter((call) => call.method === 'PUT')).toHaveLength(0);
  });

  it('refuses two different new passwords', async () => {
    const { user, calls } = await mount();
    await type(user, CURRENT, NEXT, `${NEXT}!`);
    await submit(user);
    expect(await screen.findByText(Ar['password.error.mismatch'])).toBeInTheDocument();
    expect(screen.getByLabelText(Ar['password.confirm'])).toHaveFocus();
    expect(calls.filter((call) => call.method === 'PUT')).toHaveLength(0);
  });

});

describe('refused password changes', () => {
  it('shows a wrong current password on its own field and clears every field', async () => {
    const { user } = await mount({
      passwordReply: () => backendFailure('INVALID_CURRENT_PASSWORD', 'كلمة السر الحالية غير صحيحة.', 403),
    });
    await type(user, 'wrong password 1', NEXT, NEXT);
    await submit(user);
    expect(await screen.findByText('كلمة السر الحالية غير صحيحة.')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByLabelText(Ar['password.current'])).toHaveFocus();
    });
    expect(screen.getByLabelText(Ar['password.current'])).toHaveValue('');
    expect(screen.getByLabelText(Ar['password.new'])).toHaveValue('');
    expect(screen.getByLabelText(Ar['password.confirm'])).toHaveValue('');
  });

  it('counts down when the server throttles guesses of the current password', async () => {
    const { user, calls } = await mount({
      passwordReply: () =>
        backendFailure('INVALID_CURRENT_PASSWORD', 'انتظر قليلًا.', 403, { 'Retry-After': '30' }),
    });
    await type(user, 'wrong password 1', NEXT, NEXT);
    await submit(user);
    const locked = await screen.findByRole('button', { name: Ar['throttle.retryIn'].replace('{time}', isolate('0:30')) });
    expect(locked).toBeDisabled();
    expect(calls.filter((call) => call.method === 'PUT')).toHaveLength(1);
  });

  it('puts a breached password on the new-password field', async () => {
    const { user } = await mount({
      passwordReply: () => backendFailure('PASSWORD_BREACHED', 'ظهرت كلمة السر هذه في تسريب.', 422),
    });
    await type(user, CURRENT, NEXT, NEXT);
    await submit(user);
    expect(await screen.findByText('ظهرت كلمة السر هذه في تسريب.')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByLabelText(Ar['password.new'])).toHaveFocus();
    });
  });

  it('shows the server field messages, for example when the new password equals the old one', async () => {
    const { user } = await mount({
      passwordReply: () =>
        envelope(
          {
            Success: false,
            Error: { Code: 'VALIDATION_FAILED', Message: 'x', Details: [{ Field: 'NewPassword', Message: 'يجب أن تختلف كلمة السر الجديدة.' }] },
          },
          422,
        ),
    });
    await type(user, CURRENT, NEXT, NEXT);
    await submit(user);
    expect(await screen.findByText('يجب أن تختلف كلمة السر الجديدة.')).toBeInTheDocument();
  });

  it('shows any other failure in an alert and keeps the slide open', async () => {
    const { user, router } = await mount({ passwordReply: () => backendFailure('INTERNAL', 'حدث خطأ في السيرفر.', 500) });
    await type(user, CURRENT, NEXT, NEXT);
    await submit(user);
    expect(await screen.findByText('حدث خطأ في السيرفر.')).toBeInTheDocument();
    expect(router.state.location.href).toBe('/account/security?slide=account-password');
  });

  it('can be cancelled without sending anything', async () => {
    const { user, calls, router } = await mount();
    await user.click(screen.getByRole('button', { name: Ar['password.change.cancel'] }));
    await waitFor(() => {
      expect(router.state.location.href).toBe('/account/security');
    });
    expect(calls.filter((call) => call.method === 'PUT')).toHaveLength(0);
  });
});

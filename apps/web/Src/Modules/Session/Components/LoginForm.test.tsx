import { act, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { envelope, stubFetch } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { LoginForm } from '@/Modules/Session/Components/LoginForm';
import { isolate } from '@/I18n/Translate';

const advance = (ms: number): void => {
  vi.advanceTimersByTime(ms);
};

const lockedReply = (status: number, retryAfter: string) =>
  new Response(JSON.stringify({ Success: false, Error: { Code: 'INVALID_CREDENTIALS', Message: 'x' } }), {
    status,
    headers: { 'Content-Type': 'application/json', 'Retry-After': retryAfter },
  });

const mountLocked = async (status = 401, retryAfter = '30') => {
  const { calls } = stubFetch(() => lockedReply(status, retryAfter));
  const view = renderWithProviders(<LoginForm onSignedIn={vi.fn()} />, { advanceTimers: advance });
  await view.user.type(screen.getByLabelText(Ar['field.email']), 'sabry@future.dev');
  await view.user.type(screen.getByLabelText(Ar['field.password']), 'a wrong password');
  await view.user.click(screen.getByRole('button', { name: Ar['login.submit'] }));
  await screen.findByRole('timer');
  return { ...view, calls };
};

const lockedButton = (time: string) => screen.getByRole('button', { name: Ar['throttle.retryIn'].replace('{time}', isolate(time)) });

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
});

describe('sign-in throttle', () => {
  it('shows a countdown from Retry-After, keeps the button off and ends the lock on time', async () => {
    const { calls } = await mountLocked();
    expect(screen.getByRole('timer')).toHaveTextContent('0:30');
    expect(screen.getByText(Ar['login.locked.title'])).toBeInTheDocument();
    expect(lockedButton('0:30')).toBeDisabled();
    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(screen.getByRole('timer')).toHaveTextContent('0:20');
    expect(lockedButton('0:20')).toBeDisabled();
    expect(calls).toHaveLength(1);
    act(() => {
      vi.advanceTimersByTime(20_000);
    });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: Ar['login.submit'] })).toBeEnabled();
    });
    expect(screen.queryByRole('timer')).not.toBeInTheDocument();
  });

  it('does not call the server again while locked, even when Enter is pressed', async () => {
    const { user, calls } = await mountLocked();
    await user.type(screen.getByLabelText(Ar['field.password']), 'another try{Enter}');
    expect(calls).toHaveLength(1);
  });

  it('announces the lock once without announcing every tick', async () => {
    await mountLocked();
    const status = screen.getByRole('status');
    expect(status).toHaveTextContent(arText('login.locked.announce', { seconds: 30 }));
    expect(screen.getByRole('timer')).toHaveAttribute('aria-live', 'off');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('also locks on a 429 reply', async () => {
    await mountLocked(429, '75');
    expect(screen.getByRole('timer')).toHaveTextContent('1:15');
  });

  it('treats a wrong password without Retry-After as an ordinary failure', async () => {
    stubFetch(() => envelope({ Success: false, Error: { Code: 'INVALID_CREDENTIALS', Message: 'خطأ.' } }, 401));
    const { user } = renderWithProviders(<LoginForm onSignedIn={vi.fn()} />, { advanceTimers: advance });
    await user.type(screen.getByLabelText(Ar['field.email']), 'sabry@future.dev');
    await user.type(screen.getByLabelText(Ar['field.password']), 'x');
    await user.click(screen.getByRole('button', { name: Ar['login.submit'] }));
    expect(await screen.findByRole('alert')).toHaveTextContent('خطأ.');
    expect(screen.queryByRole('timer')).not.toBeInTheDocument();
  });
});

describe('sign-in from the keyboard', () => {
  it('walks email, password, the show-password button and the submit button in order, and Enter submits', async () => {
    const { calls } = stubFetch(() => envelope({ Success: true, Data: { User: {}, CsrfToken: 'token', ExpiresAt: '2026-11-10T12:00:00Z' } }));
    const onSignedIn = vi.fn();
    const { user } = renderWithProviders(<LoginForm onSignedIn={onSignedIn} />, { advanceTimers: advance });
    await user.tab();
    expect(screen.getByLabelText(Ar['field.email'])).toHaveFocus();
    await user.keyboard('sabry@future.dev');
    await user.tab();
    expect(screen.getByLabelText(Ar['field.password'])).toHaveFocus();
    await user.keyboard('a long enough password');
    await user.tab();
    expect(screen.getByRole('button', { name: Ar['password.show'] })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: Ar['login.submit'] })).toHaveFocus();
    await user.tab({ shift: true });
    await user.tab({ shift: true });
    await user.keyboard('{Enter}');
    await waitFor(() => {
      expect(onSignedIn).toHaveBeenCalledOnce();
    });
    expect(calls).toHaveLength(1);
  });
});

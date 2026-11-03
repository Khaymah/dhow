import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { useOrganizationStore } from '@/Core/Store/useOrganizationStore';
import { envelope, META_BODY, meBody, ORG_ID, stubFetch, UNAUTHENTICATED_BODY } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { InvitePage } from '@/Modules/Identity/Pages/InvitePage';
import { takeInviteTokenFromHash, takeInviteTokenFromLocation } from '@/Modules/Identity/Store/InviteToken';
import { useInviteTokenStore } from '@/Modules/Identity/Store/useInviteTokenStore';
import { isolate } from '@/I18n/Translate';

const TOKEN = 'a1B2c3D4e5F6g7H8i9J0k1L2m3N4o5P6q7R8s9T0u1V';
const PASSWORD = 'a brand new passphrase 2026';

const preview = (email = 'ali@future.dev', exists = false) => ({
  Success: true,
  Data: {
    OrganizationName: 'Future Solution',
    InviterName: 'Mona Adel',
    Email: email,
    Role: 'developer',
    ExpiresAt: '2026-11-03T08:00:00Z',
    AccountExists: exists,
  },
});

const accepted = {
  Success: true,
  Data: {
    User: { Id: 'u', Email: 'ali@future.dev', Name: 'Ali Hassan', Locale: 'ar' },
    Organization: { Id: ORG_ID, Name: 'Future Solution', Slug: 'future-solution', CreatedAt: '2026-10-01T08:00:00Z' },
    Role: 'developer',
  },
};

interface IInviteBackend {
  previewReply?: () => Response;
  signedIn?: boolean;
  email?: string;
}

const backend = ({ previewReply, signedIn = false, email = 'ali@future.dev' }: IInviteBackend = {}) =>
  stubFetch((request) => {
    const { pathname } = new URL(request.url);
    if (pathname.endsWith('/invitations/preview')) return previewReply ? previewReply() : envelope(preview());
    if (pathname.endsWith('/me/invitations/accept')) return envelope(accepted);
    if (pathname.endsWith('/invitations/accept')) return envelope(accepted);
    if (pathname.endsWith('/meta')) return envelope(META_BODY);
    if (pathname.endsWith('/me')) {
      if (!signedIn) return envelope(UNAUTHENTICATED_BODY, 401);
      const me = meBody();
      me.Data.User.Email = email;
      return envelope(me);
    }
    return envelope({ Success: false, Error: { Code: 'NOT_FOUND', Message: 'x' } }, 404);
  });

const mount = () => {
  const onSignIn = vi.fn();
  const onCreated = vi.fn();
  const onJoined = vi.fn();
  const view = renderWithProviders(<InvitePage onSignIn={onSignIn} onCreated={onCreated} onJoined={onJoined} />);
  return { ...view, onSignIn, onCreated, onJoined };
};

afterEach(() => {
  useInviteTokenStore.getState().Clear();
  useOrganizationStore.setState({ CurrentId: null });
});

describe('the invitation token', () => {
  it('is taken from the hash and removed from the address at once', () => {
    window.history.replaceState(null, '', `/invite#${TOKEN}`);
    takeInviteTokenFromLocation();
    expect(useInviteTokenStore.getState().Token).toBe(TOKEN);
    expect(window.location.hash).toBe('');
    expect(window.location.pathname).toBe('/invite');
    expect(takeInviteTokenFromHash('#not valid!')).toBe(true);
    expect(useInviteTokenStore.getState().Token).toBe(TOKEN);
  });

  it('never ends up in the address after the router strips it', async () => {
    backend();
    const router = createAppRouter(createMemoryHistory({ initialEntries: [`/invite#${TOKEN}`] }));
    render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
    expect(await screen.findByText(Ar['invite.page.title'].replace('{inviter}', isolate('Mona Adel')).replace('{organization}', isolate('Future Solution')))).toBeInTheDocument();
    expect(router.state.location.href).toBe('/invite');
  });
});

describe('the invitation page', () => {
  it('asks for the link when there is no token', () => {
    backend();
    mount();
    expect(screen.getByText(Ar['invite.page.missing.title'])).toBeInTheDocument();
  });

  it('creates the account with a name and a strong password, then sends the person to sign in', async () => {
    useInviteTokenStore.getState().Set(TOKEN);
    const { calls } = backend();
    const { user, onCreated } = mount();
    expect(await screen.findByDisplayValue('ali@future.dev')).toHaveAttribute('readonly');
    await user.type(screen.getByLabelText(Ar['invite.page.name']), 'Ali Hassan');
    await user.type(screen.getByLabelText(Ar['field.password']), PASSWORD);
    expect(screen.getByRole('meter', { name: Ar['password.strength.label'] })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['invite.page.submit'] }));
    expect(await screen.findByText(Ar['invite.page.done.title'].replace('{organization}', isolate('Future Solution')))).toBeInTheDocument();
    const accept = calls.find((call) => call.url.endsWith('/invitations/accept'));
    expect(await accept?.clone().json()).toEqual({ Token: TOKEN, Name: 'Ali Hassan', Password: PASSWORD, Locale: 'ar' });
    await user.click(screen.getByRole('button', { name: Ar['invite.page.done.action'] }));
    expect(onCreated).toHaveBeenCalled();
  });

  it('refuses a short password before calling the server', async () => {
    useInviteTokenStore.getState().Set(TOKEN);
    const { calls } = backend();
    const { user } = mount();
    await user.type(await screen.findByLabelText(Ar['invite.page.name']), 'Ali');
    await user.type(screen.getByLabelText(Ar['field.password']), 'short');
    await user.click(screen.getByRole('button', { name: Ar['invite.page.submit'] }));
    expect(await screen.findByText(Ar['password.error.tooShort'])).toBeInTheDocument();
    expect(calls.some((call) => call.url.endsWith('/invitations/accept'))).toBe(false);
  });

  it('says plainly when the invitation was used, revoked or expired', async () => {
    useInviteTokenStore.getState().Set(TOKEN);
    backend({
      previewReply: () =>
        envelope({ Success: false, Error: { Code: 'INVITATION_INVALID', Message: 'رابط الدعوة هذا لم يعد يعمل.' } }, 410),
    });
    mount();
    expect(await screen.findByText(Ar['invite.page.invalid.title'])).toBeInTheDocument();
    expect(screen.getByText('رابط الدعوة هذا لم يعد يعمل.')).toBeInTheDocument();
  });

  it('sends an existing account to sign in first', async () => {
    useInviteTokenStore.getState().Set(TOKEN);
    backend({ previewReply: () => envelope(preview('ali@future.dev', true)) });
    const { user, onSignIn } = mount();
    await user.click(await screen.findByRole('button', { name: Ar['invite.page.signIn'] }));
    expect(onSignIn).toHaveBeenCalled();
    expect(screen.queryByLabelText(Ar['field.password'])).not.toBeInTheDocument();
  });

  it('joins a signed-in account with the same email', async () => {
    useInviteTokenStore.getState().Set(TOKEN);
    const { calls } = backend({ signedIn: true });
    const { user, onJoined } = mount();
    await user.click(await screen.findByRole('button', { name: Ar['invite.page.join'].replace('{organization}', isolate('Future Solution')) }));
    await waitFor(() => {
      expect(onJoined).toHaveBeenCalled();
    });
    expect(calls.some((call) => call.url.endsWith('/me/invitations/accept'))).toBe(true);
  });

  it('never joins a signed-in account with another email, and offers to sign out', async () => {
    useInviteTokenStore.getState().Set(TOKEN);
    const { calls } = backend({ signedIn: true, email: 'eve@future.dev' });
    mount();
    expect(await screen.findByText(Ar['invite.page.mismatch.title'].replace('{email}', isolate('ali@future.dev')))).toBeInTheDocument();
    expect(screen.getByRole('button', { name: Ar['invite.page.signOut'].replace('{email}', isolate('eve@future.dev')) })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /انضم/ })).not.toBeInTheDocument();
    expect(calls.some((call) => call.url.includes('/accept'))).toBe(false);
  });
});

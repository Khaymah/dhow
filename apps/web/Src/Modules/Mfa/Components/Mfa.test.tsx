import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { startBackend, type IBackendOptions } from '@/Core/Testing/Backend';
import { GOOD_CODE, GOOD_PASSWORD, NEW_CODES, passkeyFixture } from '@/Core/Testing/MfaBackend';
import { useToastStore } from '@/Core/Store/useToastStore';
import { findPage } from '@/Core/Testing/Pages';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const mount = async (options: IBackendOptions = {}, url = '/account/security') => {
  const backend = startBackend(options);
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  const dialog = await findPage(Ar['shell.nav.security']);
  await within(dialog).findByRole('list', { name: Ar['mfa.section.title'] });
  return { ...backend, router, user, dialog };
};

const posted = (calls: Request[], path: string) => calls.filter((call) => call.method === 'POST' && call.url.endsWith(path));

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('two-step verification in my account', () => {
  it('shows that nothing is on yet, with the glossary terms', async () => {
    const { dialog } = await mount();
    const section = within(dialog).getByRole('list', { name: Ar['mfa.section.title'] });
    expect(within(section).getByText(Ar['mfa.totp.title'])).toBeInTheDocument();
    expect(within(section).getByText(Ar['mfa.totp.off.alone'])).toBeInTheDocument();
    expect(within(section).queryByText(Ar['mfa.totp.off'])).not.toBeInTheDocument();
    expect(within(section).getByText(Ar['mfa.recovery.off'])).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['mfa.passkeys.empty'])).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['mfa.passkeys.unsupported'])).toBeInTheDocument();
  });

  it('turns TOTP on with the QR, the code and the password, then shows the recovery codes once', async () => {
    const { user, router, calls } = await mount();
    await user.click(screen.getByRole('button', { name: Ar['mfa.totp.enable'] }));
    const slide = await screen.findByRole('dialog', { name: Ar['totp.slide.title'] });
    expect(router.state.location.href).toBe('/account/security?slide=account-totp');
    await user.click(await within(slide).findByRole('button', { name: Ar['totp.start'] }));
    expect(await within(slide).findByRole('img', { name: Ar['totp.qr'] })).toBeInTheDocument();
    expect(within(slide).getByText('JBSW Y3DP EHPK 3PXP JBSW Y3DP EHPK 3PXP')).toBeInTheDocument();
    expect(within(slide).queryByRole('group', { name: Ar['factor.methods'] })).not.toBeInTheDocument();

    await user.type(within(slide).getByLabelText(Ar['totp.code']), GOOD_CODE);
    await user.type(within(slide).getByLabelText(Ar['factor.method.password']), 'not my password at all');
    await user.click(screen.getByRole('button', { name: Ar['totp.submit'] }));
    expect(await within(slide).findByText('INVALID_CURRENT_FACTOR')).toBeInTheDocument();

    await user.type(within(slide).getByLabelText(Ar['factor.method.password']), GOOD_PASSWORD);
    await user.click(screen.getByRole('button', { name: Ar['totp.submit'] }));
    const codes = await within(slide).findByRole('list', { name: Ar['mfa.recovery.list'] });
    expect(within(codes).getAllByRole('listitem')).toHaveLength(10);
    expect(within(codes).getByText(NEW_CODES[0] ?? '')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['totp.submit'] })).not.toBeInTheDocument();
    const confirm = posted(calls, '/me/totp/confirm').at(-1);
    expect(await confirm?.clone().json()).toEqual({ Code: GOOD_CODE, CurrentFactor: { Method: 'password', Password: GOOD_PASSWORD } });

    await user.click(within(slide).getByRole('button', { name: Ar['mfa.recovery.done'] }));
    await waitFor(() => {
      expect(router.state.location.href).toBe('/account/security');
    });
    expect(await screen.findByText(arText('mfa.recovery.remaining', { count: 10 }))).toBeInTheDocument();
  });

  it('needs a current second factor to turn TOTP off', async () => {
    const { user, calls } = await mount({ mfa: { TotpEnabled: true, RecoveryCodesRemaining: 4, Passkeys: [] } });
    await user.click(screen.getByRole('button', { name: Ar['mfa.totp.disable'] }));
    expect(screen.getByText(Ar['mfa.totp.disable.question'])).toBeInTheDocument();
    const methods = screen.getByRole('group', { name: Ar['factor.methods'] });
    expect(within(methods).getAllByRole('button').map((b) => b.textContent)).toEqual([Ar['factor.method.totp'], Ar['factor.method.recovery_code']]);
    await user.click(screen.getByRole('button', { name: Ar['mfa.totp.disable.confirm'] }));
    expect(await screen.findByText(Ar['factor.error.required'])).toBeInTheDocument();
    expect(posted(calls, '/me/totp/disable')).toHaveLength(0);

    await user.type(screen.getByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(screen.getByRole('button', { name: Ar['mfa.totp.disable.confirm'] }));
    expect(await screen.findByText(Ar['mfa.totp.disabled'])).toBeInTheDocument();
    expect(await screen.findByText(Ar['mfa.totp.off.alone'])).toBeInTheDocument();
  });

  it('makes new recovery codes after a confirmation and offers copy and download', async () => {
    const { user } = await mount({ mfa: { TotpEnabled: true, RecoveryCodesRemaining: 2, Passkeys: [] } });
    expect(screen.getByText(arText('mfa.recovery.remaining', { count: 2 }))).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['mfa.recovery.regenerate'] }));
    await user.type(screen.getByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(screen.getByRole('button', { name: Ar['mfa.recovery.regenerate.confirm'] }));
    const codes = await screen.findByRole('list', { name: Ar['mfa.recovery.list'] });
    expect(within(codes).getAllByRole('listitem')).toHaveLength(10);
    await user.click(screen.getByRole('button', { name: Ar['mfa.recovery.copy'] }));
    expect(await navigator.clipboard.readText()).toBe(NEW_CODES.join('\n'));
    expect(screen.getByRole('button', { name: Ar['mfa.recovery.download'] })).toBeEnabled();
  });

  it('renames and removes a passkey, and removing needs a factor', async () => {
    const { user, state } = await mount({ mfa: { TotpEnabled: true, RecoveryCodesRemaining: 5, Passkeys: [passkeyFixture()] } });
    expect(screen.getByText('MacBook Touch ID')).toBeInTheDocument();
    expect(screen.getByText(Ar['mfa.passkeys.synced'])).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: Ar['mfa.passkeys.rename.aria'].replace('{name}', isolate('MacBook Touch ID')) }));
    const name = screen.getByRole('textbox', { name: Ar['mfa.passkeys.name'] });
    await user.clear(name);
    await user.type(name, 'YubiKey 5C');
    await user.click(screen.getByRole('button', { name: Ar['mfa.passkeys.rename.save'] }));
    expect(await screen.findByText('YubiKey 5C')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: Ar['mfa.passkeys.remove.aria'].replace('{name}', isolate('YubiKey 5C')) }));
    await user.type(screen.getByLabelText(Ar['factor.method.totp']), '000000');
    await user.click(screen.getByRole('button', { name: Ar['mfa.passkeys.remove.confirm'] }));
    expect(await screen.findByText('INVALID_CURRENT_FACTOR')).toBeInTheDocument();
    expect(state.mfa.Passkeys).toHaveLength(1);
    await user.type(screen.getByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(screen.getByRole('button', { name: Ar['mfa.passkeys.remove.confirm'] }));
    expect(await screen.findByText(Ar['mfa.passkeys.removed'])).toBeInTheDocument();
    expect(state.mfa.Passkeys).toHaveLength(0);
  });
});

describe('starting TOTP and the TOTP status line', () => {
  it('calls passkeys enough only when the account has one', async () => {
    const { dialog } = await mount({ mfa: { TotpEnabled: false, RecoveryCodesRemaining: 10, Passkeys: [passkeyFixture()] } });
    const section = within(dialog).getByRole('list', { name: Ar['mfa.section.title'] });
    expect(await within(section).findByText(Ar['mfa.totp.off'])).toBeInTheDocument();
    expect(within(section).queryByText(Ar['mfa.totp.off.alone'])).not.toBeInTheDocument();
  });

  it('does not start an enrollment just because the TOTP slide was opened from a link', async () => {
    const { calls } = startBackend();
    const user = userEvent.setup();
    render(<App router={createAppRouter(createMemoryHistory({ initialEntries: ['/account/security?slide=account-totp'] }))} queryClient={createQueryClient({ retry: false })} />);
    const slide = await screen.findByRole('dialog', { name: Ar['totp.slide.title'] });
    const start = await within(slide).findByRole('button', { name: Ar['totp.start'] });
    expect(within(slide).getByText(Ar['totp.start.about'])).toBeInTheDocument();
    expect(posted(calls, '/me/totp')).toHaveLength(0);
    expect(within(slide).queryByRole('img', { name: Ar['totp.qr'] })).not.toBeInTheDocument();
    await user.click(start);
    expect(await within(slide).findByRole('img', { name: Ar['totp.qr'] })).toBeInTheDocument();
    expect(posted(calls, '/me/totp')).toHaveLength(1);
  });
});

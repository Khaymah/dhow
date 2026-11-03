import { screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure, startBackend, type IBackendOptions } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';
import { GOOD_CODE } from '@/Core/Testing/MfaBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import {
  configuredSettings,
  settingsFixture,
  settingsRoute,
  type ISettingsBackend,
  type ISettingsReplies,
  type TSettingsFixture,
} from '@/Core/Testing/SettingsBackend';
import { Ar } from '@/I18n/Locales/Ar';
import { SettingsBody } from '@/Modules/Settings/Components/SettingsBody';
import { isolate } from '@/I18n/Translate';

const TOTP = { TotpEnabled: true, RecoveryCodesRemaining: 10, Passkeys: [] };

const mount = (settings: TSettingsFixture, replies: ISettingsReplies = {}, options: IBackendOptions = {}) => {
  const state: ISettingsBackend = { settings, puts: [], tests: 0 };
  const backend = startBackend({ ...options, instanceOwner: true, routes: [settingsRoute(state, replies)] });
  const view = renderWithProviders(<SettingsBody onAddProvider={() => undefined} onEditProvider={() => undefined} />);
  return { ...backend, ...view, settings: state };
};

const card = async (title: string): Promise<HTMLElement> => {
  const heading = await screen.findByRole('heading', { level: 3, name: title });
  const section = heading.closest('section');
  if (section === null) throw new Error(`no card for ${title}`);
  return section;
};

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the server settings page', () => {
  it('shows one loading state per card while the single request runs', async () => {
    mount(settingsFixture(), { get: () => new Promise<Response>(() => undefined) as unknown as Response });
    expect(await screen.findAllByRole('status', { name: Ar['settings.loading'] })).toHaveLength(5);
  });

  it('shows the error with a retry that loads the settings', async () => {
    let failed = false;
    const { user } = mount(settingsFixture(), {
      get: () => {
        if (failed) return envelope({ Success: true, Data: settingsFixture() });
        failed = true;
        return backendFailure('INTERNAL', 'x', 500);
      },
    });
    const retries = await screen.findAllByRole('button', { name: Ar['settings.retry'] });
    expect(screen.getAllByText(Ar['settings.error'])).toHaveLength(5);
    const [first] = retries;
    if (first === undefined) throw new Error('no retry button');
    await user.click(first);
    expect(await screen.findByText(Ar['settings.mail.off'])).toBeInTheDocument();
  });

  it('says what is off when nothing is set yet', async () => {
    mount(settingsFixture());
    const mail = await card(Ar['settings.mail.title']);
    expect(await within(mail).findByText(Ar['settings.mail.off'])).toBeInTheDocument();
    expect(within(mail).getByText(Ar['settings.secret.notSaved'])).toBeInTheDocument();
    expect(within(mail).getByRole('button', { name: Ar['settings.mail.test'] })).toBeDisabled();
    expect(within(mail).getByText(Ar['settings.mail.testNeedsHost'])).toBeInTheDocument();
    expect(within(await card(Ar['settings.logs.title'])).getByText(Ar['settings.allDefault'])).toBeInTheDocument();
    expect(within(mail).getByRole('button', { name: Ar['settings.save'] })).toBeDisabled();
  });

  it('shows saved values, a saved secret without its value and a setting locked by the environment', async () => {
    mount(configuredSettings());
    const mail = await card(Ar['settings.mail.title']);
    expect(await within(mail).findByDisplayValue('smtp.example.com')).toBeInTheDocument();
    expect(within(mail).getByText(Ar['settings.secret.saved'])).toBeInTheDocument();
    expect(within(mail).queryByText(Ar['settings.mail.off'])).toBeNull();
    const limits = await card(Ar['settings.rateLimits.title']);
    expect(within(limits).getByText(Ar['settings.lockedByEnv'])).toBeInTheDocument();
    expect(within(limits).getByText('DHOW_RATE_LIMIT_PUBLIC')).toBeInTheDocument();
    expect(within(limits).getByText('1000')).toBeInTheDocument();
    expect(within(limits).queryByDisplayValue('1000')).toBeNull();
  });

});

describe('saving server settings', () => {
  it('saves only the changed fields after a fresh second factor', async () => {
    const { user, settings } = mount(configuredSettings(), {}, { mfa: TOTP });
    const mail = await card(Ar['settings.mail.title']);
    const port = await within(mail).findByLabelText(new RegExp(Ar['settings.mail.port']));
    await user.clear(port);
    await user.type(port, '2525');
    await user.click(within(mail).getByRole('button', { name: Ar['settings.save'] }));
    expect(await within(mail).findByText(Ar['settings.confirm.question'])).toBeInTheDocument();
    await user.type(await within(mail).findByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(within(mail).getByRole('button', { name: Ar['settings.mail.confirm'] }));
    await waitFor(() => {
      expect(useToastStore.getState().Toasts.some((toast) => toast.Title === Ar['settings.saved'])).toBe(true);
    });
    expect(settings.puts).toEqual([
      { path: '/api/v1/instance/settings/mail', body: { Port: 2525, Version: 2, CurrentFactor: { Method: 'totp', Code: GOOD_CODE } } },
    ]);
  });

  it('clears a saved secret with an explicit empty value', async () => {
    const { user, settings } = mount(configuredSettings(), {}, { mfa: TOTP });
    const mail = await card(Ar['settings.mail.title']);
    await user.click(await within(mail).findByRole('button', { name: Ar['settings.secret.clear'] }));
    expect(within(mail).getByText(Ar['settings.secret.willClear'])).toBeInTheDocument();
    expect(within(mail).getByText(Ar['settings.secret.notSaved'])).toBeInTheDocument();
    await user.click(within(mail).getByRole('button', { name: Ar['settings.save'] }));
    await user.type(await within(mail).findByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(within(mail).getByRole('button', { name: Ar['settings.mail.confirm'] }));
    await waitFor(() => {
      expect(settings.puts).toHaveLength(1);
    });
    expect(settings.puts[0]?.body).toMatchObject({ Password: '' });
  });

  it('asks the owner to turn on a second factor before any save', async () => {
    const { user, settings } = mount(settingsFixture());
    const orgs = await card(Ar['settings.organizations.title']);
    await user.selectOptions(await within(orgs).findByRole('combobox'), 'any-user');
    await user.click(within(orgs).getByRole('button', { name: Ar['settings.save'] }));
    expect(await within(orgs).findByText(Ar['settings.factor.needed'])).toBeInTheDocument();
    expect(settings.puts).toEqual([]);
  });

  it('shows the reason of a refused value under its field', async () => {
    const { user } = mount(configuredSettings(), {
      put: () =>
        envelope({ Success: false, Error: { Code: 'SETTING_INVALID', Message: 'x', Details: [{ Field: 'Port', Message: 'اكتب منفذًا من 1 إلى 65535.' }] } }, 400),
    }, { mfa: TOTP });
    const mail = await card(Ar['settings.mail.title']);
    const port = await within(mail).findByLabelText(new RegExp(Ar['settings.mail.port']));
    await user.clear(port);
    await user.type(port, '70000');
    await user.click(within(mail).getByRole('button', { name: Ar['settings.save'] }));
    await user.type(await within(mail).findByLabelText(Ar['factor.method.totp']), GOOD_CODE);
    await user.click(within(mail).getByRole('button', { name: Ar['settings.mail.confirm'] }));
    expect(await within(mail).findByText('اكتب منفذًا من 1 إلى 65535.')).toBeInTheDocument();
  });

  it('refuses a number field that is not a whole number before asking for a factor', async () => {
    const { user } = mount(settingsFixture(), {}, { mfa: TOTP });
    const logs = await card(Ar['settings.logs.title']);
    const days = await within(logs).findByLabelText(new RegExp(Ar['settings.logs.retention']));
    await user.clear(days);
    await user.type(days, 'ten');
    await user.click(within(logs).getByRole('button', { name: Ar['settings.save'] }));
    expect(await within(logs).findByText(Ar['settings.wholeNumber'])).toBeInTheDocument();
    expect(within(logs).queryByText(Ar['settings.confirm.question'])).toBeNull();
  });

  it('sends a test email and shows where it went, or why it failed', async () => {
    let fail = false;
    const { user, settings } = mount(configuredSettings(), {
      test: () => (fail ? backendFailure('MAIL_TEST_FAILED', 'لم يقبل خادم البريد رسالة التجربة.', 502) : envelope({ Success: true, Data: { SentTo: 'sabry@future.dev' } })),
    });
    const mail = await card(Ar['settings.mail.title']);
    await user.click(await within(mail).findByRole('button', { name: Ar['settings.mail.test'] }));
    expect(await within(mail).findByText(Ar['settings.mail.testSent'].replace('{email}', isolate('sabry@future.dev')))).toBeInTheDocument();
    fail = true;
    await user.click(within(mail).getByRole('button', { name: Ar['settings.mail.test'] }));
    expect(await within(mail).findByText('لم يقبل خادم البريد رسالة التجربة.')).toBeInTheDocument();
    expect(settings.tests).toBe(2);
  });
});

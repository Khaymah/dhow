import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { backendFailure } from '@/Core/Testing/Backend';
import { findPage } from '@/Core/Testing/Pages';
import {
  API,
  mountTargets,
  PASSWORD,
  TARGET_URL,
} from '@/Core/Testing/TargetsManageHarness';
import {
  FINGERPRINT,
  newTargetsManageState,
  OTHER_FINGERPRINT,
  KEY_MATERIAL,
  PUBLIC_KEY,
} from '@/Core/Testing/TargetsManageBackend';
import { HOST_ONE, HOST_TWO } from '@/Core/Testing/TargetsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const hostRow = async (position: number): Promise<HTMLElement> => {
  const main = await findPage('web');
  const list = await within(main).findByRole('list', { name: Ar['targets.hosts.title'] });
  const row = within(list).getAllByRole('listitem')[position - 1];
  if (!row) throw new Error(`no host row ${String(position)}`);
  return row;
};

const panelOf = async (question: string): Promise<HTMLElement> => {
  const panel = (await screen.findByText(question)).parentElement;
  if (!panel) throw new Error('the question has no panel');
  return panel;
};

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the servers of a target', () => {
  it('shows each server with its pinned fingerprint or a warning that the key is not pinned', async () => {
    mountTargets({ url: TARGET_URL });
    const first = await hostRow(1);
    const second = await hostRow(2);

    expect(within(first).getByText(Ar['targets.hostkey.badge.pinned'])).toBeInTheDocument();
    expect(within(first).getByText(FINGERPRINT)).toBeInTheDocument();
    expect(within(second).getByText(Ar['targets.hostkey.badge.unpinned'])).toBeInTheDocument();
    expect(within(second).getByText(Ar['targets.hostkey.unpinned'])).toBeInTheDocument();
  });

  it('hides every change from a maintainer, who can only read the target', async () => {
    mountTargets({ url: TARGET_URL, role: 'maintainer' });
    const second = await hostRow(2);
    expect(within(second).queryByRole('button', { name: Ar['targets.hostkey.fetch'] })).not.toBeInTheDocument();
    expect(within(second).queryByRole('button', { name: Ar['targets.host.edit'] })).not.toBeInTheDocument();
    const main = await findPage('web');
    expect(within(main).getByText(Ar['targets.manage.readOnly'])).toBeInTheDocument();
    expect(within(main).queryByRole('button', { name: Ar['targets.form.save'] })).not.toBeInTheDocument();
    expect(within(main).getByRole('button', { name: Ar['targets.check.button'] })).toBeInTheDocument();
  });
});

describe('pinning the key of a server', () => {
  it('fetches the key, shows its fingerprint, and pins only after the comparison is ticked and the second step is given', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const row = await hostRow(2);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.hostkey.fetch'] }));

    expect(await within(row).findByText(arText('targets.hostkey.pending.title'))).toBeInTheDocument();
    expect(within(row).getByText(FINGERPRINT)).toBeInTheDocument();
    expect(view.manage.writes[0]).toMatchObject({ Method: 'POST', Path: `${API}/targets/web/hosts/${HOST_TWO}/fetch-host-key` });

    const confirm = within(row).getByRole('button', { name: Ar['targets.hostkey.pending.confirm'] });
    expect(confirm).toBeDisabled();
    await view.user.type(await within(row).findByLabelText(Ar['factor.method.password']), PASSWORD);
    expect(confirm).toBeDisabled();
    await view.user.click(within(row).getByRole('checkbox', { name: Ar['targets.hostkey.pending.check'] }));
    expect(confirm).toBeEnabled();
    expect(view.manage.writes).toHaveLength(1);
    await view.user.click(confirm);

    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(2);
    });
    expect(view.manage.writes[1]).toEqual({
      Method: 'POST',
      Path: `${API}/targets/web/hosts/${HOST_TWO}/confirm-host-key`,
      Body: { Fingerprint: FINGERPRINT, CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
    expect(await screen.findByText(arText('targets.hostkey.pinned.toast', { host: 'web-2.example.com' }))).toBeInTheDocument();
    await waitFor(() => {
      expect(within(row).getByText(Ar['targets.hostkey.badge.pinned'])).toBeInTheDocument();
    });
    expect(within(row).queryByText(Ar['targets.hostkey.pending.title'])).not.toBeInTheDocument();
  });

  it('opens the paste box when the server will not connect to a private address, and shows the key that was pasted', async () => {
    const manage = newTargetsManageState();
    manage.fetchFailure = backendFailure('TARGET_HOST_BLOCKED', 'هذا العنوان خاص. الصق المفتاح بدل جلبه.', 422);
    const view = mountTargets({ url: TARGET_URL, manage });
    const row = await hostRow(2);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.hostkey.fetch'] }));

    expect(await within(row).findByText('هذا العنوان خاص. الصق المفتاح بدل جلبه.')).toBeInTheDocument();
    const box = await within(row).findByLabelText(Ar['targets.hostkey.paste.label']);
    await view.user.type(box, 'web-2.example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey');
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.hostkey.paste.submit'] }));

    await waitFor(() => {
      expect(view.manage.writes.at(-1)).toEqual({
        Method: 'PUT',
        Path: `${API}/targets/web/hosts/${HOST_TWO}/pending-host-key`,
        Body: { HostKey: 'web-2.example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey' },
      });
    });
    expect(await within(row).findByText(FINGERPRINT)).toBeInTheDocument();
    expect(within(row).queryByLabelText(Ar['targets.hostkey.paste.label'])).not.toBeInTheDocument();
  });

  it('keeps the confirmation open and says why when the key on the server changed after it was fetched', async () => {
    const manage = newTargetsManageState();
    const view = mountTargets({ url: TARGET_URL, manage });
    const row = await hostRow(2);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.hostkey.fetch'] }));
    await within(row).findByText(FINGERPRINT);
    const hosts = manage.target.Hosts as Record<string, unknown>[];
    manage.target = { ...manage.target, Hosts: hosts.map((host) => (host.Id === HOST_TWO ? { ...host, PendingHostKeyFingerprint: OTHER_FINGERPRINT } : host)) };

    await view.user.type(await within(row).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(row).getByRole('checkbox', { name: Ar['targets.hostkey.pending.check'] }));
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.hostkey.pending.confirm'] }));

    expect(await within(row).findByText('البصمة لا تطابق.')).toBeInTheDocument();
    expect(within(row).getByText(Ar['targets.hostkey.badge.unpinned'])).toBeInTheDocument();
  });

  it('asks again for the comparison when a new key replaces the waiting one', async () => {
    const manage = newTargetsManageState();
    const view = mountTargets({ url: TARGET_URL, manage });
    const row = await hostRow(2);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.hostkey.fetch'] }));
    await within(row).findByText(FINGERPRINT);
    await view.user.click(within(row).getByRole('checkbox', { name: Ar['targets.hostkey.pending.check'] }));
    manage.fingerprintOnFetch = OTHER_FINGERPRINT;
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.hostkey.fetch'] }));
    await within(row).findByText(OTHER_FINGERPRINT);

    expect(within(row).getByRole('checkbox', { name: Ar['targets.hostkey.pending.check'] })).not.toBeChecked();
    expect(within(row).getByRole('button', { name: Ar['targets.hostkey.pending.confirm'] })).toBeDisabled();
  });
});

describe('the servers list', () => {
  it('moves a server with the second step and sends the whole new order', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const row = await hostRow(2);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.host.moveUp'] }));
    await view.user.type(await within(row).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.host.move.confirm'] }));

    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(1);
    });
    expect(view.manage.writes[0]).toEqual({
      Method: 'PUT',
      Path: `${API}/targets/web/hosts-order`,
      Body: { Version: 4, HostIds: [HOST_TWO, HOST_ONE], CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
  });

  it('warns that a new address drops the pinned key before it saves the address', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const row = await hostRow(1);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.host.edit'] }));
    expect(within(row).getByText(Ar['targets.host.edit.question'])).toBeInTheDocument();
    const address = within(row).getByLabelText(Ar['targets.form.host']);
    await view.user.clear(address);
    await view.user.type(address, 'web-9.example.com');
    await view.user.type(await within(row).findByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(row).getByRole('button', { name: Ar['targets.host.edit.confirm'] }));

    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(1);
    });
    expect(view.manage.writes[0]).toEqual({
      Method: 'PUT',
      Path: `${API}/targets/web/hosts/${HOST_ONE}`,
      Body: { Version: 4, Host: 'web-9.example.com', Port: 22, CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
  });

  it('adds a server last with the second step', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const main = await findPage('web');
    await view.user.click(await within(main).findByRole('button', { name: Ar['targets.host.add'] }));
    await view.user.type(await within(main).findByLabelText(Ar['targets.form.host']), '203.0.113.9');
    const panel = await panelOf(Ar['targets.host.add.question']);
    await view.user.type(within(panel).getByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(panel).getByRole('button', { name: Ar['targets.host.add.confirm'] }));
    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(1);
    });
    expect(view.manage.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/targets/web/hosts`,
      Body: { Version: 4, Host: '203.0.113.9', Port: 22, CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
  });
});

describe('the deploy key', () => {
  it('writes the private key to the secret once, clears the box and never shows it again', async () => {
    const manage = newTargetsManageState({ KeySecretName: 'WEB_SSH_KEY' });
    const view = mountTargets({ url: TARGET_URL, manage });
    const main = await findPage('web');
    expect(await within(main).findByText(arText('targets.key.secret', { name: 'WEB_SSH_KEY' }))).toBeInTheDocument();

    const box = within(main).getByLabelText(Ar['targets.keySecret.value']);
    await view.user.click(box);
    await view.user.paste(KEY_MATERIAL);
    await view.user.click(within(main).getByRole('button', { name: Ar['targets.keySecret.submit'] }));

    await waitFor(() => {
      expect(view.manage.secrets).toEqual([{ Name: 'WEB_SSH_KEY', Value: KEY_MATERIAL }]);
    });
    expect(await screen.findByText(arText('targets.keySecret.saved', { name: 'WEB_SSH_KEY' }))).toBeInTheDocument();
    await waitFor(() => {
      expect(box).toHaveValue('');
    });
    expect(document.body.innerHTML).not.toContain('fake-key-material');
    expect(window.location.href).not.toContain('fake-key-material');
    expect(JSON.stringify([Object.entries(window.localStorage), Object.entries(window.sessionStorage)])).not.toContain('fake-key-material');
    expect(within(main).queryByRole('button', { name: Ar['targets.keySecret.attach.confirm'] })).not.toBeInTheDocument();
  });

  it('asks for the second step to make a new secret the key of the target', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const main = await findPage('web');
    await view.user.click(await within(main).findByLabelText(Ar['targets.keySecret.value']));
    await view.user.paste(KEY_MATERIAL);
    await view.user.click(within(main).getByRole('button', { name: Ar['targets.keySecret.submit'] }));

    const panel = await panelOf(arText('targets.keySecret.attach.question', { name: 'WEB_SSH_KEY' }));
    await view.user.type(within(panel).getByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(panel).getByRole('button', { name: Ar['targets.keySecret.attach.confirm'] }));
    await waitFor(() => {
      expect(view.manage.writes).toHaveLength(2);
    });
    expect(view.manage.secrets[0]?.Name).toBe('WEB_SSH_KEY');
    expect(view.manage.writes[1]).toEqual({
      Method: 'PUT',
      Path: `${API}/targets/web`,
      Body: { Version: 4, KeySecretName: 'WEB_SSH_KEY', CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
  });

  it('shows the public key the server made, with a copy button, and none of the private half', async () => {
    const view = mountTargets({ url: TARGET_URL });
    const main = await findPage('web');
    await view.user.click(await within(main).findByRole('button', { name: Ar['targets.key.generate'] }));
    const panel = await panelOf(Ar['targets.key.generate.question']);
    await view.user.type(within(panel).getByLabelText(Ar['factor.method.password']), PASSWORD);
    await view.user.click(within(panel).getByRole('button', { name: Ar['targets.key.generate.confirm'] }));

    expect(await within(main).findByText(PUBLIC_KEY)).toBeInTheDocument();
    expect(within(main).getByRole('button', { name: Ar['targets.key.copy'] })).toBeInTheDocument();
    expect(view.manage.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/targets/web/generate-key`,
      Body: { CurrentFactor: { Method: 'password', Password: PASSWORD } },
    });
  });
});

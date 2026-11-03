import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, ORG_ID, stubFetch, USER_ID } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';
import { RunnerBody } from '@/Modules/Runners/Components/RunnerBody';
import { RunnerNewBody } from '@/Modules/Runners/Components/RunnerNewBody';
import { isolate } from '@/I18n/Translate';

const RUNNER_ID = '0198a000-0000-7000-8000-0000000000e1';
const SECRET = 'dhow_rrt_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcd';
const COMMAND = 'sudo -u dhow-runner dhow-runner register --url https://dhow.example.com --token-file -';
const path = (request: Request) => new URL(request.url).pathname;

const runner = (over: Record<string, unknown> = {}) => ({
  Id: RUNNER_ID,
  OrganizationId: ORG_ID,
  Name: 'build-01',
  Labels: ['linux-builders'],
  CapabilityLabels: ['dhow.os=linux', 'dhow.arch=amd64', 'dhow.isolation=container'],
  Os: 'linux',
  Arch: 'amd64',
  Version: 'v0.3.0',
  IsolationLevels: ['container'],
  Capacity: 2,
  State: 'active',
  Status: 'offline',
  KeyVersion: 1,
  KeysRotatedAt: '2026-10-28T08:00:00Z',
  CreatedAt: '2026-10-28T08:00:00Z',
  ...over,
});

const detail = (role: string, handler: (request: Request) => Response | Promise<Response>) => {
  const { calls } = stubFetch((request) => (path(request).endsWith('/me') ? envelope(meBody(role)) : handler(request)));
  return { ...renderWithProviders(<RunnerBody runnerId={RUNNER_ID} />), calls };
};

describe('a runner', () => {
  it('shows a skeleton, then the facts', async () => {
    detail('developer', () => envelope({ Success: true, Data: runner() }));
    expect(screen.getByRole('status', { name: Ar['runners.loading'] })).toBeInTheDocument();
    expect(await screen.findByText(Ar['runners.about.offline'])).toBeInTheDocument();
    expect(screen.getByRole('list', { name: Ar['runners.facts.capabilities'] })).toHaveTextContent('dhow.isolation=container');
    expect(screen.getByText('linux/amd64')).toBeInTheDocument();
    expect(screen.getByText(arText('runners.facts.capacity.value', { count: 2 }))).toBeInTheDocument();
    expect(screen.getByText(Ar['runners.detail.readOnly'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['runners.revoke.action'] })).not.toBeInTheDocument();
  });

  it('says so when the runner is not in the organization', async () => {
    detail('owner', () => backendFailure('NOT_FOUND', 'x', 404));
    expect(await screen.findByText(Ar['runners.detail.missing'])).toBeInTheDocument();
  });

  it('shows other errors with a retry', async () => {
    detail('owner', () => backendFailure('INTERNAL', 'وقع خطأ في السيرفر.', 500));
    expect(await screen.findByText('وقع خطأ في السيرفر.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: Ar['runners.retry'] })).toBeInTheDocument();
  });

  it('renames a runner and changes its labels', async () => {
    const { user, calls } = detail('owner', (request) =>
      request.method === 'PATCH'
        ? envelope({ Success: true, Data: runner({ Name: 'build-main', Labels: ['linux-builders', 'region=eu'] }) })
        : envelope({ Success: true, Data: runner() }),
    );
    const name = await screen.findByLabelText(Ar['runners.edit.name']);
    await user.clear(name);
    await user.type(name, 'build-main');
    await user.type(screen.getByRole('textbox', { name: Ar['runners.edit.labels'] }), ' region=eu');
    await user.click(screen.getByRole('button', { name: Ar['runners.edit.save'] }));
    await waitFor(() => {
      expect(calls.some((call) => call.method === 'PATCH')).toBe(true);
    });
    const sent = calls.find((call) => call.method === 'PATCH');
    expect(await sent?.clone().json()).toEqual({ Name: 'build-main', Labels: ['linux-builders', 'region=eu'] });
  });

  it('refuses a reserved label before asking the server', async () => {
    const { user, calls } = detail('owner', () => envelope({ Success: true, Data: runner() }));
    await user.type(await screen.findByRole('textbox', { name: Ar['runners.edit.labels'] }), ' dhow.os=linux');
    await user.click(screen.getByRole('button', { name: Ar['runners.edit.save'] }));
    expect(await screen.findByText(Ar['runners.new.error.labels'].replace('{prefix}', isolate('dhow.')))).toBeInTheDocument();
    expect(calls.some((call) => call.method === 'PATCH')).toBe(false);
  });

  it('pauses a runner', async () => {
    const { user, calls } = detail('admin', (request) =>
      request.method === 'PATCH'
        ? envelope({ Success: true, Data: runner({ State: 'disabled', Status: 'disabled', DisabledReason: 'manual' }) })
        : envelope({ Success: true, Data: runner() }),
    );
    await user.click(await screen.findByRole('button', { name: Ar['runners.pause.action'] }));
    expect(await screen.findByRole('button', { name: Ar['runners.resume.action'] })).toBeInTheDocument();
    const sent = calls.find((call) => call.method === 'PATCH');
    expect(await sent?.clone().json()).toEqual({ Disabled: true });
  });

  it('revokes a runner only after a second click, then hides the actions', async () => {
    const { user, calls } = detail('owner', (request) =>
      request.method === 'POST'
        ? envelope({ Success: true, Data: runner({ State: 'revoked', Status: 'revoked', RevokedAt: '2026-10-28T22:00:00Z' }) })
        : envelope({ Success: true, Data: runner() }),
    );
    await user.click(await screen.findByRole('button', { name: Ar['runners.revoke.action'] }));
    expect(calls.some((call) => call.method === 'POST')).toBe(false);
    await user.click(screen.getByRole('button', { name: Ar['runners.revoke.confirm'] }));
    expect(await screen.findByText(Ar['runners.about.revoked'])).toBeInTheDocument();
    expect(path(calls.find((call) => call.method === 'POST') ?? new Request('http://x'))).toBe(
      `/api/v1/organizations/${ORG_ID}/runners/${RUNNER_ID}/revoke`,
    );
    expect(screen.queryByLabelText(Ar['runners.edit.name'])).not.toBeInTheDocument();
  });
});

const created = {
  RegistrationToken: {
    Id: '0198a000-0000-7000-8000-0000000000f1',
    Description: 'Build box',
    Labels: ['linux-builders', 'region=eu'],
    MaxUses: 3,
    Uses: 0,
    ExpiresAt: '2026-10-29T04:00:00Z',
    Status: 'active',
    CreatedAt: '2026-10-28T22:00:00Z',
    CreatedByUserId: USER_ID,
  },
  Token: SECRET,
  RegisterCommand: COMMAND,
};

const newToken = (role: string, create: () => Response) => {
  const { calls } = stubFetch((request) => (path(request).endsWith('/me') ? envelope(meBody(role)) : create()));
  const onDone = vi.fn();
  return { ...renderWithProviders(<RunnerNewBody onDone={onDone} />), calls, onDone };
};

describe('adding a runner', () => {
  it('creates a token with labels, uses and expiry, then shows it once with the command', async () => {
    const { user, calls, onDone, client } = newToken('owner', () => envelope({ Success: true, Data: created }, 201));
    await user.type(await screen.findByLabelText(Ar['runners.new.description']), 'Build box');
    await user.type(screen.getByLabelText(Ar['runners.new.labels']), 'linux-builders, region=eu linux-builders');
    const uses = screen.getByLabelText(Ar['runners.new.uses']);
    await user.clear(uses);
    await user.type(uses, '3');
    await user.click(screen.getByRole('radio', { name: Ar['runners.new.expiry.6'] }));
    await user.click(screen.getByRole('button', { name: Ar['runners.new.submit'] }));
    expect(await screen.findByLabelText(Ar['runners.created.token'])).toHaveTextContent(SECRET);
    expect(screen.getByLabelText(Ar['runners.command.label'])).toHaveTextContent(COMMAND);
    const sent = calls.find((call) => call.method === 'POST');
    expect(path(sent ?? new Request('http://x'))).toBe(`/api/v1/organizations/${ORG_ID}/runner-registration-tokens`);
    expect(await sent?.clone().json()).toEqual({ Description: 'Build box', Labels: ['linux-builders', 'region=eu'], MaxUses: 3, ExpiresInSeconds: 21600 });
    expect(JSON.stringify(client.getQueryCache().getAll().map((query) => query.state.data))).not.toContain(SECRET);
    expect(client.getMutationCache().getAll()).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: Ar['runners.created.done'] }));
    expect(onDone).toHaveBeenCalledOnce();
  });

  it('checks the labels and the uses before asking the server', async () => {
    const { user, calls } = newToken('owner', () => envelope({ Success: true, Data: created }, 201));
    await user.type(await screen.findByLabelText(Ar['runners.new.labels']), 'Production');
    const uses = screen.getByLabelText(Ar['runners.new.uses']);
    await user.clear(uses);
    await user.type(uses, '51');
    await user.click(screen.getByRole('button', { name: Ar['runners.new.submit'] }));
    expect(await screen.findByText(Ar['runners.new.error.labels'].replace('{prefix}', isolate('dhow.')))).toBeInTheDocument();
    expect(screen.getByText(Ar['runners.new.error.uses'].replace('{max}', isolate('50')))).toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST')).toBe(false);
  });

  it('shows the server refusal and keeps the form', async () => {
    const { user } = newToken('owner', () => backendFailure('SECOND_FACTOR_REQUIRED', 'فعّل الخطوة الثانية أولًا.', 403));
    await user.type(await screen.findByLabelText(Ar['runners.new.labels']), 'linux');
    await user.click(screen.getByRole('button', { name: Ar['runners.new.submit'] }));
    expect(await screen.findByText('فعّل الخطوة الثانية أولًا.')).toBeInTheDocument();
    expect(screen.getByLabelText(Ar['runners.new.labels'])).toHaveValue('linux');
  });

  it('refuses roles that cannot manage runners', async () => {
    newToken('maintainer', () => envelope({ Success: true, Data: created }, 201));
    expect(await screen.findByText(Ar['runners.new.forbidden'])).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: Ar['runners.new.submit'] })).not.toBeInTheDocument();
  });
});

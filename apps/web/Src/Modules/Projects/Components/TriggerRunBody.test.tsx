import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { backendFailure } from '@/Core/Testing/Backend';
import { envelope, meBody, stubFetch } from '@/Core/Testing/Fetch';
import { notLinked, ok, PROJECT_ID, projectFixture, RUN_ID, runFixture, SHA } from '@/Core/Testing/ProjectsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { TriggerRunBody } from '@/Modules/Projects/Components/TriggerRunBody';

const path = (request: Request) => new URL(request.url).pathname;

const INPUTS = {
  Known: true,
  Enabled: true,
  Ref: 'refs/heads/main',
  Sha: SHA,
  Inputs: [
    { Name: 'dry', Type: 'boolean', Required: false, Options: [], Description: 'Plan only' },
    { Name: 'note', Type: 'string', Required: true, Options: [] },
    { Name: 'target', Type: 'choice', Required: true, Options: ['staging', 'production'], Default: 'staging' },
  ],
};

const mount = (role: string, inputs: () => Response = () => ok(INPUTS), project = projectFixture({ CallerRole: role })) => {
  const bodies: unknown[] = [];
  stubFetch(async (request) => {
    const p = path(request);
    if (p.endsWith('/me')) return envelope(meBody(role));
    if (request.method === 'POST' && p.endsWith('/runs')) {
      bodies.push(await request.json());
      return ok(runFixture({ State: 'pending' }), 201);
    }
    if (p.endsWith('/manual-inputs')) return inputs();
    if (p.endsWith(`/projects/${PROJECT_ID}`)) return ok(project);
    return backendFailure('NOT_FOUND', 'x', 404);
  });
  const onStarted = vi.fn();
  return { ...renderWithProviders(<TriggerRunBody projectId={PROJECT_ID} onStarted={onStarted} />), bodies, onStarted };
};

describe('starting a run by hand', () => {
  it('types every input, validates it and sends only what was given', async () => {
    const view = mount('developer');
    const ref = await screen.findByRole('textbox', { name: Ar['runs.trigger.ref'] });
    expect(ref).toHaveValue('main');
    expect(screen.getByText('Plan only')).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runs.trigger.submit'] }));
    expect(await screen.findByText(Ar['runs.trigger.error.required'])).toBeInTheDocument();
    expect(view.bodies).toHaveLength(0);

    await view.user.type(screen.getByRole('textbox', { name: /note/ }), 'release');
    await view.user.click(screen.getByRole('checkbox', { name: 'dry' }));
    await view.user.selectOptions(screen.getByRole('combobox', { name: /target/ }), 'production');
    await view.user.type(screen.getByRole('textbox', { name: Ar['runs.trigger.sha'] }), 'xyz');
    await view.user.click(screen.getByRole('button', { name: Ar['runs.trigger.submit'] }));
    expect(await screen.findByText(Ar['runs.trigger.error.sha'])).toBeInTheDocument();
    await view.user.clear(screen.getByRole('textbox', { name: Ar['runs.trigger.sha'] }));
    await view.user.click(screen.getByRole('button', { name: Ar['runs.trigger.submit'] }));
    await waitFor(() => {
      expect(view.onStarted).toHaveBeenCalledWith(RUN_ID);
    });
    expect(view.bodies[0]).toEqual({ Ref: 'main', Inputs: { dry: true, note: 'release', target: 'production' } });
  });

  it('explains why it cannot start', async () => {
    const viewer = mount('viewer');
    expect(await screen.findByText(Ar['runs.trigger.forbidden'])).toBeInTheDocument();
    viewer.unmount();
    const off = mount('owner', () => ok({ ...INPUTS, Enabled: false, Inputs: [] }));
    expect(await screen.findByText(/on\.manual/)).toBeInTheDocument();
    off.unmount();
    const unlinked = mount('owner', notLinked);
    expect(await screen.findByText(Ar['runs.trigger.notLinked'])).toBeInTheDocument();
    unlinked.unmount();
    mount('owner', () => ok(INPUTS), projectFixture({ State: 'archived', CallerRole: 'owner' }));
    expect(await screen.findByText(Ar['runs.trigger.archived'])).toBeInTheDocument();
  });

  it('still starts by branch when no file was read yet, and shows the server refusal', async () => {
    let refuse = true;
    stubFetch(async (request) => {
      const p = path(request);
      if (p.endsWith('/me')) return envelope(meBody('owner'));
      if (request.method === 'POST') {
        await request.text();
        return refuse ? backendFailure('REF_HEAD_UNKNOWN', 'لم يصل إشعار لهذا الفرع بعد.', 422) : ok(runFixture(), 201);
      }
      if (p.endsWith('/manual-inputs')) return ok({ Known: false, Enabled: false, Ref: 'refs/heads/main', Inputs: [] });
      return ok(projectFixture({ CallerRole: 'owner' }));
    });
    const onStarted = vi.fn();
    const view = renderWithProviders(<TriggerRunBody projectId={PROJECT_ID} onStarted={onStarted} />);
    expect(await screen.findByText(Ar['runs.trigger.unknown'])).toBeInTheDocument();
    await view.user.click(screen.getByRole('button', { name: Ar['runs.trigger.submit'] }));
    expect(await screen.findByText('لم يصل إشعار لهذا الفرع بعد.')).toBeInTheDocument();
    refuse = false;
    await view.user.click(screen.getByRole('button', { name: Ar['runs.trigger.submit'] }));
    await waitFor(() => {
      expect(onStarted).toHaveBeenCalledOnce();
    });
  });
});

import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { cachedText, fillForm, mount, openNew, PAGE_URL, PROJECT_NAME, storedText, submit, typePassword } from '@/Core/Testing/IngestHarness';
import { newIngestState, NEW_TOKEN_VALUE, type IIngestState } from '@/Core/Testing/IngestBackend';
import { findPage } from '@/Core/Testing/Pages';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

const SLIDE_URL = `${PAGE_URL}?slide=ingest-token-new`;

const failure = (code: string, message: string, status: number): Response =>
  new Response(JSON.stringify({ Success: false, Error: { Code: code, Message: message } }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });

const protectedState = (): IIngestState => {
  const state = newIngestState();
  state.protectedReach = ['production'];
  return state;
};

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the slide gate', () => {
  it.each(['developer', 'viewer'])('shows a %s no form and sends no request when the slide is opened by its link', async (role) => {
    const view = mount(SLIDE_URL, role);
    await findPage(PROJECT_NAME);
    await waitFor(() => {
      expect(view.router.state.location.search).not.toHaveProperty('slide');
    });
    expect(screen.queryByLabelText(Ar['ingest.form.name'])).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog', { name: Ar['ingest.new.title'] })).not.toBeInTheDocument();
    expect(view.state.writes).toEqual([]);
    expect(view.calls.some((call) => call.method !== 'GET' && call.url.includes('/ingest-tokens'))).toBe(false);
    expect(view.calls.some((call) => call.url.includes('/ingest-tokens'))).toBe(false);
  });

  it('refuses an owner whose project role is only a developer', async () => {
    const view = mount(SLIDE_URL, 'owner', newIngestState(), { CallerRole: 'developer' });
    await findPage(PROJECT_NAME);
    await waitFor(() => {
      expect(view.router.state.location.search).not.toHaveProperty('slide');
    });
    expect(screen.queryByLabelText(Ar['ingest.form.name'])).not.toBeInTheDocument();
    expect(view.calls.some((call) => call.url.includes('/ingest-tokens'))).toBe(false);
  });

  it('shows the archived reason and no form on an archived project', async () => {
    const view = mount(SLIDE_URL, 'owner', newIngestState(), { State: 'archived' });
    const dialog = await screen.findByRole('dialog', { name: Ar['ingest.new.title'] });
    expect(await within(dialog).findByText(Ar['projects.archived.blocked'])).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(Ar['ingest.form.name'])).not.toBeInTheDocument();
    expect(view.state.writes).toEqual([]);
  });

  it('still opens for a maintainer', async () => {
    mount(SLIDE_URL, 'maintainer');
    const dialog = await screen.findByRole('dialog', { name: Ar['ingest.new.title'] });
    expect(await within(dialog).findByLabelText(Ar['ingest.form.name'])).toBeInTheDocument();
  });
});

describe('errors after the reach prompt', () => {
  const reachPrompted = async (state: IIngestState) => {
    const view = mount(PAGE_URL, 'owner', state);
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'wide', '**');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByText(Ar['ingest.reach.title']);
    await view.user.click(within(dialog).getByRole('checkbox', { name: new RegExp(Ar['ingest.reach.acknowledge']) }));
    return { view, dialog };
  };

  it('shows a wrong second factor after the person ticked the box, and keeps the prompt', async () => {
    const state = protectedState();
    const { view, dialog } = await reachPrompted(state);
    state.failCreate = failure('INVALID_CURRENT_FACTOR', 'كلمة المرور غير صحيحة', 403);
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByText('كلمة المرور غير صحيحة')).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['ingest.reach.title'])).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(Ar['ingest.created.value'])).not.toBeInTheDocument();
  });

  it('shows a name that is already taken after the person ticked the box', async () => {
    const state = protectedState();
    const { view, dialog } = await reachPrompted(state);
    state.failCreate = failure('INGEST_TOKEN_NAME_TAKEN', 'الاسم مستخدم', 409);
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByText('الاسم مستخدم')).toBeInTheDocument();
  });

  it('does not show the first notice next to the prompt', async () => {
    const { dialog } = await reachPrompted(protectedState());
    expect(within(dialog).queryByText('تحتاج إلى تأكيد')).not.toBeInTheDocument();
  });

  it('shows the generic error and no prompt when the server names no environment', async () => {
    const state = newIngestState();
    state.emptyReach = true;
    const view = mount(PAGE_URL, 'owner', state);
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'wide', '**');
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByText('تحتاج إلى تأكيد')).toBeInTheDocument();
    expect(within(dialog).queryByText(Ar['ingest.reach.title'])).not.toBeInTheDocument();
    expect(within(dialog).queryByRole('checkbox')).not.toBeInTheDocument();
  });
});

describe('the value on screen', () => {
  it('shows the value before the list refreshes, even when the refresh never answers', async () => {
    const state = newIngestState();
    const view = mount(PAGE_URL, 'owner', state);
    const dialog = await openNew(view);
    await within(await findPage(PROJECT_NAME)).findByText('github-main');
    state.holdTokens = new Promise<void>(() => undefined);
    await fillForm(view, dialog, 'ci', 'docs/*');
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByLabelText(Ar['ingest.created.value'])).toHaveTextContent(NEW_TOKEN_VALUE);
    expect(within(dialog).queryByLabelText(Ar['ingest.form.name'])).not.toBeInTheDocument();
  });

  it('leaves no live form with the box ticked after a successful create', async () => {
    const view = mount(PAGE_URL, 'owner', protectedState());
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'wide', '**');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByText(Ar['ingest.reach.title']);
    await view.user.click(within(dialog).getByRole('checkbox', { name: new RegExp(Ar['ingest.reach.acknowledge']) }));
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByLabelText(Ar['ingest.created.value']);
    expect(within(dialog).queryByRole('checkbox')).not.toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: Ar['ingest.form.submit'] })).not.toBeInTheDocument();
    expect(view.state.writes).toHaveLength(2);
  });

  it('removes the value from the page the moment the slide is closed with Escape', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'ci', 'docs/*');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByLabelText(Ar['ingest.created.value']);

    await view.user.keyboard('{Escape}');
    expect(document.body.innerHTML).not.toContain(NEW_TOKEN_VALUE);
    expect(screen.queryByLabelText(Ar['ingest.created.value'])).not.toBeInTheDocument();
    expect(storedText()).not.toContain(NEW_TOKEN_VALUE);
    expect(cachedText(view)).not.toContain(NEW_TOKEN_VALUE);
  });

  it('removes the value from the page the moment the browser goes back', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'ci', 'docs/*');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByLabelText(Ar['ingest.created.value']);

    view.router.history.back();
    await waitFor(() => {
      expect(view.router.state.location.search).not.toHaveProperty('slide');
    });
    expect(document.body.innerHTML).not.toContain(NEW_TOKEN_VALUE);
  });
});

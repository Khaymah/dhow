import { configure, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { newIngestState, NEW_TOKEN_VALUE, type IIngestState } from '@/Core/Testing/IngestBackend';
import { API, cachedText, fillForm, mount, openNew, PAGE_URL, PASSWORD, PROJECT_NAME, storedText, submit, typePassword } from '@/Core/Testing/IngestHarness';
import { findPage } from '@/Core/Testing/Pages';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('making a token', () => {
  it('sends the name, the patterns, the days and the second step, then shows the value once', async () => {
    const view = mount();
    const dialog = await openNew(view);
    expect(view.router.state.location.href).toBe(`${PAGE_URL}?slide=ingest-token-new`);
    await fillForm(view, dialog, 'ci-release', 'release/*');
    await view.user.selectOptions(within(dialog).getByLabelText(Ar['ingest.patterns.kind']), 'tag');
    await typePassword(view, dialog);
    await submit(view, dialog);

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]).toEqual({
      Method: 'POST',
      Path: `${API}/ingest-tokens`,
      Body: {
        Name: 'ci-release',
        RefPatterns: [{ Kind: 'tag', Pattern: 'release/*' }],
        ExpiresInDays: 90,
        AcknowledgeProtectedReach: false,
        CurrentFactor: { Method: 'password', Password: PASSWORD },
      },
    });
    const value = await within(dialog).findByLabelText(Ar['ingest.created.value']);
    expect(value).toHaveTextContent(NEW_TOKEN_VALUE);
    expect(within(dialog).getByText(Ar['ingest.created.title'])).toBeInTheDocument();
    expect(within(dialog).getByText(Ar['ingest.created.lead'])).toBeInTheDocument();
    expect(view.router.state.location.href).not.toContain(NEW_TOKEN_VALUE);
    expect(storedText()).not.toContain(NEW_TOKEN_VALUE);
    expect(cachedText(view)).not.toContain(NEW_TOKEN_VALUE);
  });

});

describe('the token value', () => {
  it('copies the value and clears it everywhere when the person closes the view', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'ci-main', 'main');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByLabelText(Ar['ingest.created.value']);

    await view.user.click(within(dialog).getByRole('button', { name: Ar['ingest.created.copy'] }));
    expect(await navigator.clipboard.readText()).toBe(NEW_TOKEN_VALUE);
    expect(await screen.findByText(Ar['ingest.created.copied'])).toBeInTheDocument();
    expect(JSON.stringify(useToastStore.getState().Toasts)).not.toContain(NEW_TOKEN_VALUE);

    await view.user.click(within(dialog).getByRole('button', { name: Ar['ingest.created.done'] }));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(document.body.innerHTML).not.toContain(NEW_TOKEN_VALUE);
    expect(storedText()).not.toContain(NEW_TOKEN_VALUE);
    expect(cachedText(view)).not.toContain(NEW_TOKEN_VALUE);
    expect(view.router.state.location.href).toBe(PAGE_URL);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText('ci-main')).toBeInTheDocument();
  });

  it('clears the value when the person leaves the page without closing the view', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'ci-main', 'main');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByLabelText(Ar['ingest.created.value']);

    await view.router.navigate({ to: '/projects/$projectId/environments', params: { projectId: PROJECT_ID } });
    await waitFor(() => {
      expect(screen.queryByLabelText(Ar['ingest.created.value'])).not.toBeInTheDocument();
    });
    expect(document.body.innerHTML).not.toContain(NEW_TOKEN_VALUE);
    expect(storedText()).not.toContain(NEW_TOKEN_VALUE);

    await view.router.navigate({ to: '/projects/$projectId/ingest', params: { projectId: PROJECT_ID }, search: { slide: 'ingest-token-new' } });
    const again = await screen.findByRole('dialog', { name: Ar['ingest.new.title'] });
    expect(within(again).queryByLabelText(Ar['ingest.created.value'])).not.toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain(NEW_TOKEN_VALUE);
  });

});

describe('refused and invalid tokens', () => {
  it('does not show a value when the server refuses', async () => {
    const view = mount();
    view.state.failCreate = new Response(JSON.stringify({ Success: false, Error: { Code: 'INGEST_TOKEN_NAME_TAKEN', Message: 'الاسم مستخدم' } }), {
      status: 409,
      headers: { 'Content-Type': 'application/json' },
    });
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'github-main', 'main');
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByText('الاسم مستخدم')).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(Ar['ingest.created.value'])).not.toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain(NEW_TOKEN_VALUE);
  });

  it.each([
    ['a name', '', 'main', '90', 'ingest.form.error.name'],
    ['a pattern', 'ci', '', '90', 'ingest.form.error.patterns'],
    ['a pattern with a space', 'ci', 'ma in', '90', 'ingest.form.error.pattern'],
    ['more than 365 days', 'ci', 'main', '400', 'ingest.form.error.days'],
    ['zero days', 'ci', 'main', '0', 'ingest.form.error.days'],
  ] as const)('refuses %s before it sends anything', async (_label, name, pattern, days, key) => {
    const view = mount();
    const dialog = await openNew(view);
    await fillForm(view, dialog, name, pattern);
    const field = within(dialog).getByLabelText(Ar['ingest.form.days']);
    await view.user.clear(field);
    await view.user.type(field, days);
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByText(Ar[key])).toBeInTheDocument();
    expect(view.state.writes).toEqual([]);
  });

  it('sends a custom number of days and every pattern once', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'ci', 'main');
    await view.user.click(within(dialog).getByRole('button', { name: Ar['ingest.patterns.add'] }));
    await view.user.type(within(dialog).getAllByLabelText(Ar['ingest.patterns.pattern'])[1] ?? document.body, 'main');
    const field = within(dialog).getByLabelText(Ar['ingest.form.days']);
    await view.user.clear(field);
    await view.user.type(field, '365');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await waitFor(() => {
      expect(view.state.writes).toHaveLength(1);
    });
    expect(view.state.writes[0]?.Body).toMatchObject({ ExpiresInDays: 365, RefPatterns: [{ Kind: 'branch', Pattern: 'main' }] });
  });
});

describe('a token that reaches protected environments', () => {
  const protectedState = (): IIngestState => {
    const state = newIngestState();
    state.protectedReach = ['production'];
    return state;
  };

  it('shows the environments, asks for an explicit yes and then resends with the acknowledgement', async () => {
    const view = mount(PAGE_URL, 'owner', protectedState());
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'wide', '**');
    await typePassword(view, dialog);
    await submit(view, dialog);

    expect(await within(dialog).findByText(Ar['ingest.reach.title'])).toBeInTheDocument();
    expect(within(dialog).getByText('production')).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(Ar['ingest.created.value'])).not.toBeInTheDocument();
    expect(view.state.writes).toHaveLength(1);
    expect(view.state.writes[0]?.Body).toMatchObject({ AcknowledgeProtectedReach: false });
    const confirm = within(dialog).getByRole('button', { name: Ar['ingest.form.submit'] });
    expect(confirm).toBeDisabled();

    const box = within(dialog).getByRole('checkbox', { name: new RegExp(Ar['ingest.reach.acknowledge']) });
    await view.user.click(box);
    expect(box).toHaveAttribute('aria-checked', 'true');
    expect(confirm).toBeEnabled();
    await typePassword(view, dialog);
    await view.user.click(confirm);

    await waitFor(() => {
      expect(view.state.writes).toHaveLength(2);
    });
    expect(view.state.writes[1]?.Body).toMatchObject({ Name: 'wide', AcknowledgeProtectedReach: true });
    expect(await within(dialog).findByLabelText(Ar['ingest.created.value'])).toHaveTextContent(NEW_TOKEN_VALUE);
    expect(within(dialog).getByText('production')).toBeInTheDocument();
  });

  it('asks again when the person changes the patterns after the environments were shown', async () => {
    const view = mount(PAGE_URL, 'owner', protectedState());
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'wide', '**');
    await typePassword(view, dialog);
    await submit(view, dialog);
    await within(dialog).findByText(Ar['ingest.reach.title']);
    await view.user.click(within(dialog).getByRole('checkbox', { name: new RegExp(Ar['ingest.reach.acknowledge']) }));

    await view.user.type(within(dialog).getByLabelText(Ar['ingest.patterns.pattern']), 'x');
    expect(within(dialog).queryByText(Ar['ingest.reach.title'])).not.toBeInTheDocument();
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByText(Ar['ingest.reach.title'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(2);
    expect(view.state.writes[1]?.Body).toMatchObject({ AcknowledgeProtectedReach: false });
    expect(within(dialog).getByRole('checkbox', { name: new RegExp(Ar['ingest.reach.acknowledge']) })).toHaveAttribute('aria-checked', 'false');
  });

  it('needs no acknowledgement when the token reaches nothing protected', async () => {
    const view = mount();
    const dialog = await openNew(view);
    await fillForm(view, dialog, 'docs', 'docs/*');
    await typePassword(view, dialog);
    await submit(view, dialog);
    expect(await within(dialog).findByLabelText(Ar['ingest.created.value'])).toBeInTheDocument();
    expect(view.state.writes).toHaveLength(1);
  });
});

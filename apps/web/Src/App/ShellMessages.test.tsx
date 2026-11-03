import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { apiUp } from '@/Core/Testing/Fetch';
import { Ar } from '@/I18n/Locales/Ar';
import { En } from '@/I18n/Locales/En';
import { WorkspaceAr } from '@/I18n/Locales/Workspace/Ar';
import { useCatalogStore } from '@/I18n/useCatalogStore';
import { loadWorkspaceMessages } from '@/I18n/WorkspaceCatalog';
import { isolate } from '@/I18n/Translate';

const renderApp = (url = '/') => {
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  return { user: userEvent.setup() };
};

beforeEach(() => {
  apiUp();
});

afterEach(async () => {
  await Promise.all([loadWorkspaceMessages('ar'), loadWorkspaceMessages('en')]);
});

describe('the workspace messages', () => {
  it('load with the shell, so the shell never shows a raw message key', async () => {
    useCatalogStore.setState({ Workspace: {} });
    renderApp();
    expect(await screen.findByRole('heading', { level: 1, name: Ar['home.title'] })).toBeInTheDocument();
    expect(useCatalogStore.getState().Workspace.ar).toBeDefined();
    expect(useCatalogStore.getState().Workspace.en).toBeUndefined();
    expect(document.body.textContent).not.toContain('home.title');
  });

  it('are fetched for the new language before the shell switches to it', async () => {
    useCatalogStore.setState({ Workspace: { ar: WorkspaceAr } });
    const { user } = renderApp();
    await screen.findByRole('heading', { level: 1, name: Ar['home.title'] });
    await user.click(screen.getByRole('button', { name: Ar['shell.language.switch'].replace('{language}', isolate(Ar['language.en'])) }));
    expect(await screen.findByRole('heading', { level: 1, name: En['home.title'] })).toBeInTheDocument();
    await waitFor(() => {
      expect(document.documentElement.lang).toBe('en');
    });
    expect(screen.getByRole('navigation', { name: En['shell.nav.label'] })).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\b(home|shell|palette)\.[a-z]/);
  });

  it('are not needed by the sign-in page', async () => {
    useCatalogStore.setState({ Workspace: {} });
    apiUp(undefined, undefined, null);
    renderApp('/login');
    expect(await screen.findByRole('heading', { name: Ar['login.title'] })).toBeInTheDocument();
    expect(useCatalogStore.getState().Workspace).toEqual({});
  });
});

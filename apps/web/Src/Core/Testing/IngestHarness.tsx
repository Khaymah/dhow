import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { ingestRoute, newIngestState, type IIngestState } from '@/Core/Testing/IngestBackend';
import { findPage } from '@/Core/Testing/Pages';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { Ar } from '@/I18n/Locales/Ar';

export const PASSWORD = 'correct horse battery staple';
export const PAGE_URL = `/projects/${PROJECT_ID}/ingest`;
export const API = `/organizations/${ORG_ID}/projects/${PROJECT_ID}`;
export const PROJECT_NAME = 'Web app';

export const mount = (url = PAGE_URL, role = 'owner', state: IIngestState = newIngestState(), project: Record<string, unknown> = {}) => {
  const backend = startBackend({
    role,
    routes: [ingestRoute(state), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role, ...project })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

export type TView = ReturnType<typeof mount>;

export const openNew = async (view: TView): Promise<HTMLElement> => {
  const main = await findPage(PROJECT_NAME);
  await view.user.click(await within(main).findByRole('button', { name: Ar['ingest.tokens.new'] }));
  return screen.findByRole('dialog', { name: Ar['ingest.new.title'] });
};

export const fillForm = async (view: TView, dialog: HTMLElement, name: string, pattern: string) => {
  const nameField = await within(dialog).findByLabelText(Ar['ingest.form.name']);
  if (name !== '') await view.user.type(nameField, name);
  if (pattern !== '') await view.user.type(within(dialog).getByLabelText(Ar['ingest.patterns.pattern']), pattern);
};

export const typePassword = async (view: TView, dialog: HTMLElement) => {
  await view.user.click(await within(dialog).findByLabelText(Ar['factor.method.password']));
  await view.user.paste(PASSWORD);
};

export const submit = async (view: TView, dialog: HTMLElement) => {
  await view.user.click(within(dialog).getByRole('button', { name: Ar['ingest.form.submit'] }));
};

export const storedText = (): string =>
  [window.location.href, JSON.stringify(window.history.state), ...Object.entries(window.localStorage), ...Object.entries(window.sessionStorage)]
    .flat()
    .join('\n');

export const cachedText = (view: TView): string =>
  JSON.stringify([
    view.client.getQueryCache().getAll().map((query) => query.state.data),
    view.client.getMutationCache().getAll().map((mutation) => [mutation.state.data, mutation.state.variables]),
  ]);

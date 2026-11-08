import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { newNotificationsState, notificationsRoute, type INotificationsState } from '@/Core/Testing/NotificationsBackend';
import { findPage } from '@/Core/Testing/Pages';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

export const PASSWORD = 'correct horse battery staple';
export const PAGE_URL = `/projects/${PROJECT_ID}/notifications`;
export const API = `/organizations/${ORG_ID}/projects/${PROJECT_ID}`;
export const PROJECT_NAME = 'Web app';

export const mount = (url = PAGE_URL, role = 'owner', state: INotificationsState = newNotificationsState(), project: Record<string, unknown> = {}) => {
  const backend = startBackend({
    role,
    routes: [notificationsRoute(state), projectsRoute({ project: () => ok(projectFixture({ CallerRole: role, ...project })) })],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state };
};

export const named = (template: string, name: string): string => template.replace('{name}', isolate(name));

export const channelRow = async (main: HTMLElement, name: string): Promise<HTMLElement> => {
  const list = await within(main).findByRole('list', { name: Ar['notifications.channels.title'] });
  const row = (await within(list).findByText(name)).closest('li');
  if (row === null) throw new Error(`no channel row for ${name}`);
  return row;
};

export type TView = ReturnType<typeof mount>;

export const openNew = async (view: TView): Promise<HTMLElement> => {
  const main = await findPage(PROJECT_NAME);
  await view.user.click(await within(main).findByRole('button', { name: Ar['notifications.channels.new'] }));
  return screen.findByRole('dialog', { name: Ar['notifications.new.title'] });
};

export const fillChannel = async (view: TView, dialog: HTMLElement, kind: string, name: string, url = '') => {
  await view.user.selectOptions(await within(dialog).findByLabelText(Ar['notifications.form.kind']), kind);
  await view.user.type(within(dialog).getByLabelText(Ar['notifications.form.name']), name);
  if (url !== '') await view.user.type(within(dialog).getByLabelText(Ar['notifications.form.url']), url);
};

export const typePassword = async (view: TView, dialog: HTMLElement) => {
  await view.user.click(await within(dialog).findByLabelText(Ar['factor.method.password']));
  await view.user.paste(PASSWORD);
};

export const submit = async (view: TView, dialog: HTMLElement) => {
  await view.user.click(within(dialog).getByRole('button', { name: Ar['notifications.form.submit'] }));
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

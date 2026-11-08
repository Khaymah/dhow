import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { SECTIONS } from '@/App/Navigation/Sections';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { DOC_LINKS } from '@/Core/Help/DocLinks';
import { LATEST_RELEASE } from '@/Core/Help/Generated/ReleaseMeta';
import { HelpContentAr } from '@/Core/Help/Generated/HelpContentAr';
import { defaultPreferences, startBackend } from '@/Core/Testing/Backend';
import { Ar } from '@/I18n/Locales/Ar';
import { En } from '@/I18n/Locales/En';
import { isolate } from '@/I18n/Translate';

const mount = (url: string, options: { seen?: string | null; role?: string; instanceOwner?: boolean } = {}) => {
  const preferences = { ...defaultPreferences(), ReleaseNotesSeen: options.seen === undefined ? LATEST_RELEASE : options.seen };
  const backend = startBackend({ role: options.role, preferences, instanceOwner: options.instanceOwner });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const user = userEvent.setup();
  render(<App router={router} queryClient={createQueryClient({ retry: false })} />);
  return { ...backend, router, user };
};

const main = () => screen.getByRole('main');
const HELP_BUTTON = new RegExp(`^${Ar['help.button']}`);
const NEWS_TITLE = new RegExp(Ar['whatsNew.title'].replace(' {version}', ''));
const whatsNew = () => screen.findByRole('heading', { name: NEWS_TITLE });
const seenCalls = (calls: Request[]) => calls.filter((call) => call.method === 'PUT' && call.url.endsWith('/me/release-notes-seen'));

describe('the help button in the page header', () => {
  it('opens the documentation of the page in the language of the app', async () => {
    mount('/about');
    await screen.findByRole('heading', { level: 1, name: Ar['shell.nav.about'] });
    const link = await within(main()).findByRole('link', { name: HELP_BUTTON });
    expect(link).toHaveAttribute('href', 'https://github.com/Khaymah/dhow/blob/main/docs/guides/about-and-doctor.md');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noreferrer noopener');
    expect(link).toHaveAccessibleDescription(Ar['help.hint.about']);
  });

  it('follows the page when the user opens a detail page of a section', async () => {
    mount('/jobs/0198a000-0000-7000-8000-0000000000c1');
    const link = await screen.findByRole('link', { name: HELP_BUTTON });
    expect(link).toHaveAttribute('href', expect.stringContaining('/docs/guides/jobs.md'));
  });

  it.each(SECTIONS.filter((section) => ['/', '/about', '/help', '/notifications', '/account/preferences'].includes(section.path)))(
    '$id shows a help link with a hint',
    async (section) => {
      mount(section.path);
      const link = await screen.findByRole('link', { name: HELP_BUTTON });
      expect(link).toHaveAttribute('href', expect.stringContaining(`/docs/${DOC_LINKS[section.id] ?? 'missing'}`));
      expect(link).toHaveAccessibleDescription(Ar[`help.hint.${section.id}`]);
    },
  );
});

describe('the help page', () => {
  it('shows the groups of questions, the troubleshooting sections and the release notes', async () => {
    mount('/help');
    expect(await screen.findByRole('heading', { level: 1, name: Ar['help.title'] })).toBeInTheDocument();
    for (const group of HelpContentAr.Faq) expect(await screen.findByRole('heading', { name: group.Title })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: Ar['help.troubleshooting.title'] })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: Ar['help.releases.title'] })).toBeInTheDocument();
    expect(screen.getByText(Ar['help.offline'])).toBeInTheDocument();
  });

  it('opens and closes an answer from its question', async () => {
    const { user } = mount('/help');
    const question = HelpContentAr.Faq[0]?.Entries[0]?.Title ?? '';
    const button = await screen.findByRole('button', { name: question });
    expect(button).toHaveAttribute('aria-expanded', 'false');
    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');
    expect(document.getElementById(button.getAttribute('aria-controls') ?? '')).toBeVisible();
    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'false');
  });

  it('works without any request to the network for the content, and links to the docs outside the app', async () => {
    const { calls, user } = mount('/help');
    await screen.findByRole('button', { name: HelpContentAr.Faq[0]?.Entries[0]?.Title ?? '' });
    await user.click(screen.getByRole('button', { name: HelpContentAr.Faq[0]?.Entries[0]?.Title ?? '' }));
    const outside = calls.filter((call) => !new URL(call.url).pathname.startsWith('/api/'));
    expect(outside).toEqual([]);
    const links = within(main()).getAllByRole('link');
    const docs = links.filter((link) => link.getAttribute('href')?.startsWith('https://github.com/Khaymah/dhow/blob/main/docs/'));
    expect(docs.length).toBeGreaterThan(5);
    for (const link of docs) expect(link).toHaveAttribute('rel', 'noreferrer noopener');
  });

  it('searches the questions, with or without Arabic diacritics', async () => {
    const { user } = mount('/help');
    const box = await screen.findByRole('searchbox', { name: Ar['help.search.label'] });
    await user.type(box, 'الإِعْدَاد');
    const first = await screen.findAllByRole('article');
    expect(first.length).toBeGreaterThan(0);
    expect(first[0]).toHaveTextContent(/الإعداد/);
    await user.clear(box);
    await user.type(box, 'zzzzqqqq');
    expect(await screen.findByText(/لا توجد نتائج للبحث/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: Ar['help.search.clear'] }));
    expect(box).toHaveValue('');
    expect(await screen.findByRole('heading', { name: HelpContentAr.Faq[0]?.Title ?? '' })).toBeInTheDocument();
  });

  it('shows the English content when the app is in English', async () => {
    const { user } = mount('/help');
    await screen.findByRole('heading', { level: 1, name: Ar['help.title'] });
    await user.click(screen.getByRole('button', { name: Ar['shell.language.switch'].replace('{language}', isolate(Ar['language.en'])) }));
    expect(await screen.findByRole('heading', { level: 1, name: En['help.title'] })).toBeInTheDocument();
    await user.type(await screen.findByRole('searchbox', { name: En['help.search.label'] }), 'cookie');
    expect((await screen.findAllByRole('article')).length).toBeGreaterThan(0);
  });
});

describe('the What is new card', () => {
  it('shows the newest release notes to a user who has not seen them, and saves that they did', async () => {
    const { calls, user } = mount('/', { seen: null });
    const section = (await whatsNew()).closest('section');
    if (section === null) throw new Error('the card has no section');
    const card = within(section);
    const highlights = HelpContentAr.Releases[0]?.Sections[0]?.Title ?? '';
    expect(await card.findByRole('heading', { name: highlights })).toBeInTheDocument();
    await user.click(card.getByRole('button', { name: Ar['whatsNew.dismiss'] }));
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: NEWS_TITLE })).not.toBeInTheDocument();
    });
    const saved = seenCalls(calls);
    expect(saved).toHaveLength(1);
    expect(await saved[0]?.clone().json()).toEqual({ Version: LATEST_RELEASE });
  });

  it('does not show again for a user who has seen this version', async () => {
    mount('/', { seen: LATEST_RELEASE });
    await screen.findByRole('heading', { level: 1, name: Ar['home.title'] });
    expect(screen.queryByRole('heading', { name: NEWS_TITLE })).not.toBeInTheDocument();
  });

  it('shows again for a user who saw an older version', async () => {
    mount('/', { seen: '0.0.1' });
    expect(await whatsNew()).toBeInTheDocument();
  });

  it('keeps the card and says so when saving fails', async () => {
    const { user } = mount('/', { seen: null });
    await whatsNew();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(JSON.stringify({ Success: false, Error: { Code: 'INTERNAL', Message: 'x' } }), { status: 500 }))),
    );
    await user.click(screen.getByRole('button', { name: Ar['whatsNew.dismiss'] }));
    expect(await screen.findByText(Ar['whatsNew.saveFailed'])).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: NEWS_TITLE })).toBeInTheDocument();
  });

  it('opens the help page from the card', async () => {
    const { router, user } = mount('/', { seen: null });
    await user.click(await screen.findByRole('button', { name: Ar['whatsNew.all'] }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/help');
    });
  });
});

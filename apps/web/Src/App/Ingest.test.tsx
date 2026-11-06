import { configure, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { FILE_DIGEST, FILE_RUN_ID, ingestFileFixture, newIngestState } from '@/Core/Testing/IngestBackend';
import { mount, openNew, PAGE_URL, PROJECT_NAME } from '@/Core/Testing/IngestHarness';
import { findPage } from '@/Core/Testing/Pages';
import { PROJECT_ID } from '@/Core/Testing/ProjectsBackend';
import { arText } from '@/Core/Testing/Text';
import { Ar } from '@/I18n/Locales/Ar';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterEach(() => {
  useToastStore.setState({ Toasts: [] });
});

describe('the ingest page', () => {
  it('lists the tokens with their patterns, creator, expiry, last use, state and reach', async () => {
    mount();
    const main = await findPage(PROJECT_NAME);
    const list = await within(main).findByRole('list', { name: Ar['ingest.tokens.title'] });

    expect(within(list).getByText('github-main')).toBeInTheDocument();
    expect(within(list).getAllByText('dhow_ing_AbCd')).toHaveLength(2);
    expect(within(list).getAllByText('main')).toHaveLength(2);
    expect(within(list).getByText(Ar['ingest.token.status.active'])).toBeInTheDocument();
    expect(within(list).getByText(Ar['ingest.token.status.revoked'])).toBeInTheDocument();
    expect(within(list).getByText('staging')).toBeInTheDocument();
    expect(within(list).getAllByText(Ar['ingest.reach.none'])).toHaveLength(1);
    expect(within(list).getAllByText(new RegExp(Ar['ingest.creator.you']))).toHaveLength(2);
    expect(within(list).getByText(new RegExp(arText('ingest.token.lastUsed', { when: '' }).trim().slice(0, 8)))).toBeInTheDocument();
    expect(within(list).getAllByRole('button', { name: arText('ingest.revoke.aria', { name: 'github-main' }) })).toHaveLength(1);
    expect(within(list).queryByRole('button', { name: arText('ingest.revoke.aria', { name: 'old-ci' }) })).not.toBeInTheDocument();
  });

  it('shows the empty state of the tokens', async () => {
    const state = newIngestState();
    state.tokens = [];
    mount(PAGE_URL, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(Ar['ingest.tokens.empty'])).toBeInTheDocument();
  });

  it('tells the person when the tokens could not be loaded and loads them on retry', async () => {
    const state = newIngestState();
    state.failTokens = new Response(JSON.stringify({ Success: false, Error: { Code: 'INTERNAL', Message: 'x' } }), {
      status: 500,
      headers: { 'Content-Type': 'application/json' },
    });
    const view = mount(PAGE_URL, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(Ar['ingest.tokens.error'])).toBeInTheDocument();
    state.failTokens = null;
    await view.user.click(within(main).getAllByRole('button', { name: Ar['projects.retry'] })[0] ?? document.body);
    expect(await within(main).findByText('github-main')).toBeInTheDocument();
  });

});

describe('the received files', () => {
  it('lists the received files with the digest, ref, commit, size, token, run link and the not verified label', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    const list = await within(main).findByRole('list', { name: Ar['ingest.files.title'] });

    expect(within(list).getByText('web')).toBeInTheDocument();
    expect(within(list).getByText(`sha256:${FILE_DIGEST.slice(0, 12)}`)).toBeInTheDocument();
    expect(within(list).getByText('main')).toBeInTheDocument();
    expect(within(list).getByText('01234567')).toBeInTheDocument();
    expect(within(list).getByText(new RegExp(arText('jobs.size.mib', { n: '2.0' })))).toBeInTheDocument();
    expect(within(list).getByText(arText('ingest.file.byToken', { name: 'github-main' }))).toBeInTheDocument();
    expect(within(list).getByText(Ar['ingest.file.unverified'])).toBeInTheDocument();
    expect(within(list).queryByText(Ar['ingest.file.verified'])).not.toBeInTheDocument();
    const run = within(list).getByRole('link', { name: arText('ingest.file.runNumber', { number: 12 }) });
    expect(run).toHaveAttribute('href', `/projects/${PROJECT_ID}/runs/${FILE_RUN_ID}`);

    await view.user.click(within(list).getByText(Ar['ingest.file.provenance']));
    expect(within(list).getByText('github-actions')).toBeInTheDocument();
    expect(within(list).getByText(Ar['ingest.file.provenance.about'])).toBeInTheDocument();
  });

  it('copies the full digest and says so', async () => {
    const view = mount();
    const main = await findPage(PROJECT_NAME);
    await view.user.click(await within(main).findByRole('button', { name: arText('ingest.file.copyDigest', { name: 'web' }) }));
    expect(await navigator.clipboard.readText()).toBe(FILE_DIGEST);
    expect(await screen.findByText(Ar['ingest.file.digestCopied'])).toBeInTheDocument();
  });

  it('shows pinned-until instead of the expiry and the missing run', async () => {
    const state = newIngestState();
    state.files = [ingestFileFixture({ PinnedUntil: '2999-02-01T00:00:00Z', RunId: undefined, RunNumber: undefined })];
    mount(PAGE_URL, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(new RegExp(arText('ingest.file.pinned', { when: '' }).trim().slice(0, 6)))).toBeInTheDocument();
    expect(within(main).getByText(Ar['ingest.file.noRun'])).toBeInTheDocument();
  });

  it('pages the received files', async () => {
    const state = newIngestState();
    state.files = Array.from({ length: 25 }, (_, index) => ingestFileFixture({ Id: `0198a000-0000-7000-8000-0000000008${String(index).padStart(2, '0')}`, Name: `file-${String(index)}` }));
    const view = mount(PAGE_URL, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    const list = await within(main).findByRole('list', { name: Ar['ingest.files.title'] });
    expect(within(list).getAllByRole('listitem').filter((item) => item.tagName === 'LI' && item.querySelector('dl'))).toHaveLength(20);
    await view.user.click(within(main).getByRole('button', { name: Ar['pager.next'] }));
    expect(await within(main).findByText('file-24')).toBeInTheDocument();
  });

  it('shows the empty state of the received files', async () => {
    const state = newIngestState();
    state.files = [];
    mount(PAGE_URL, 'owner', state);
    const main = await findPage(PROJECT_NAME);
    expect(await within(main).findByText(Ar['ingest.files.empty'])).toBeInTheDocument();
  });

});

describe('the page around them', () => {
  it('shows the send-a-build panel with placeholders only and the outputs through env', async () => {
    mount();
    const main = await findPage(PROJECT_NAME);
    const panel = await within(main).findByRole('region', { name: Ar['ingest.send.group'] });
    const text = panel.textContent;
    expect(text).toContain('secrets.DHOW_INGEST_TOKEN');
    expect(text).toContain('<your-ingest-token>');
    expect(text).toContain('env:');
    expect(text).not.toContain('dhow_ing_');
    const links = within(panel).getAllByRole('link');
    expect(links.map((link) => link.getAttribute('href'))).toEqual([
      'https://github.com/Khaymah/dhow/blob/main/docs/guides/ingest.md',
      'https://github.com/Khaymah/dhow/blob/main/docs/guides/ingest-action.md',
    ]);
  });

  it('injects no style element', async () => {
    const view = mount();
    const before = document.querySelectorAll('style').length;
    await openNew(view);
    expect(document.querySelectorAll('style')).toHaveLength(before);
  });
});

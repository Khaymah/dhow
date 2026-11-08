import { screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { envelope, notFound, stubFetch } from '@/Core/Testing/Fetch';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { AboutBody } from '@/Modules/System/Components/AboutBody';

const BUILD = { Version: 'v1.4.0', Commit: '9f3c2ab', BuiltAt: '2026-11-06T09:30:00Z' };
const FIX = 'اقرأ سجل الخدمة: journalctl -u dhow -n 100';

const report = (checks?: Record<string, unknown>[]) => ({
  Success: true,
  Message: 'system.doctor.loaded',
  Data: {
    ServerBuild: BUILD,
    Health: { Status: 'ok', Checks: [{ Name: 'database', Ok: true }] },
    ...(checks === undefined ? {} : { Checks: checks }),
  },
});

const OWNER_CHECKS = [
  { Code: 'database', Status: 'ok', Title: 'فحص الجاهزية: database' },
  { Code: 'migrations', Status: 'fail', Title: 'فحص الجاهزية: migrations', Fix: FIX },
  { Code: 'mptcp', Status: 'warn', Title: 'MPTCP في نواة النظام', Fix: 'عطّل MPTCP' },
  { Code: 'clock', Status: 'info', Title: 'ساعة السيرفر' },
];

const open = (body: Record<string, unknown>) => {
  const stub = stubFetch((request) => (new URL(request.url).pathname.endsWith('/system/doctor') ? envelope(body) : notFound()));
  return { ...renderWithProviders(<AboutBody />), ...stub };
};

describe('AboutBody', () => {
  it('shows the version, the commit, the build date and the health to every user', async () => {
    open(report());
    const version = await screen.findByText('v1.4.0');
    expect(version.tagName).toBe('BDI');
    expect(version).toHaveAttribute('dir', 'ltr');
    expect(screen.getByText('9f3c2ab').tagName).toBe('BDI');
    expect(screen.getByText(Ar['about.build.date'])).toBeInTheDocument();
    expect(document.querySelector('time')).toHaveAttribute('datetime', BUILD.BuiltAt);
    expect(screen.getByText(Ar['about.build.health'])).toBeInTheDocument();
    expect(screen.getByText(Ar['status.state.live'])).toBeInTheDocument();
  });

  it('does not show the checks card to a member, because the server sends no checks', async () => {
    open(report());
    await screen.findByText('v1.4.0');
    expect(screen.queryByText(Ar['about.checks.title'])).not.toBeInTheDocument();
    expect(screen.queryByText(Ar['about.checks.hostNote'])).not.toBeInTheDocument();
  });

  it('shows every check to the owner with a word for each state and the fix under the failing ones', async () => {
    open(report(OWNER_CHECKS));
    const list = await screen.findByRole('list', { name: Ar['about.checks.title'] });
    expect(within(list).getAllByRole('listitem')).toHaveLength(OWNER_CHECKS.length);
    const rowOf = (title: string): HTMLElement => {
      const row = within(list).getByText(title).closest('li');
      if (row === null) throw new Error(`no row for ${title}`);
      return row;
    };
    expect(within(rowOf('فحص الجاهزية: database')).getByText(Ar['about.status.ok'])).toBeInTheDocument();
    const failed = rowOf('فحص الجاهزية: migrations');
    expect(within(failed).getByText(Ar['about.status.fail'])).toBeInTheDocument();
    expect(failed).toHaveTextContent(`${Ar['api.error.fix']} ${FIX}`);
    expect(within(rowOf('MPTCP في نواة النظام')).getByText(Ar['about.status.warn'])).toBeInTheDocument();
    const note = rowOf('ساعة السيرفر');
    expect(within(note).getByText(Ar['about.status.info'])).toBeInTheDocument();
    expect(within(note).queryByText(Ar['api.error.fix'])).not.toBeInTheDocument();
    expect(screen.getByText(Ar['about.checks.hostNote'])).toBeInTheDocument();
  });

  it('shows the documentation links in the language of the page and opens them in a new tab safely', async () => {
    open(report());
    const list = await screen.findByRole('list', { name: Ar['about.links.title'] });
    const links = within(list).getAllByRole('link');
    expect(links.length).toBeGreaterThanOrEqual(5);
    for (const link of links) {
      expect(link).toHaveAttribute('target', '_blank');
      expect(link).toHaveAttribute('rel', 'noreferrer noopener');
      expect(link.getAttribute('href')).toMatch(/^https:\/\/github\.com\/Khaymah\/dhow\/blob\/main\//);
    }
    expect(within(list).getByRole('link', { name: new RegExp(Ar['about.link.editions']) })).toHaveAttribute(
      'href',
      'https://github.com/Khaymah/dhow/blob/main/docs/editions-and-licensing.md',
    );
  });

  it('states the license and the commercial part', async () => {
    open(report());
    expect(await screen.findByText(Ar['about.license.body'])).toHaveTextContent('AGPL-3.0');
    expect(screen.getByText(Ar['about.license.body'])).toHaveTextContent('المميزات التجارية');
  });

  it('says the commit and the date are unknown when the binary does not carry them', async () => {
    open({ ...report(), Data: { ...report().Data, ServerBuild: { Version: 'dev', Commit: '' } } });
    await screen.findByText('dev');
    expect(screen.getAllByText(Ar['about.build.unknown'])).toHaveLength(2);
  });

  it('shows the error and keeps the links when the server cannot answer, and loads again on retry', async () => {
    let failing = true;
    stubFetch((request) => {
      if (!new URL(request.url).pathname.endsWith('/system/doctor')) return notFound();
      return failing ? envelope({ Success: false, Error: { Code: 'INTERNAL', Message: 'خطأ' } }, 500) : envelope(report());
    });
    const { user } = renderWithProviders(<AboutBody />);
    expect(await screen.findByText(Ar['about.error.title'])).toBeInTheDocument();
    expect(screen.getByRole('list', { name: Ar['about.links.title'] })).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole('button', { name: Ar['status.retry'] }));
    expect(await screen.findByText('v1.4.0')).toBeInTheDocument();
    expect(screen.queryByText(Ar['about.error.title'])).not.toBeInTheDocument();
  });
});

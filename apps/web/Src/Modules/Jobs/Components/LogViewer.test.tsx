import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ORG_ID } from '@/Core/Testing/Fetch';
import { JOB_ID } from '@/Core/Testing/JobsBackend';
import { installLayout } from '@/Core/Testing/Layout';
import { renderWithProviders } from '@/Core/Testing/Render';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';
import type { TJobLogChunk } from '@/Modules/Jobs/Api/JobsApi';
import { LogViewer, type ILogViewerProps } from '@/Modules/Jobs/Components/LogViewer';
import { appendChunks, EMPTY_LOG, type ILogBuffer } from '@/Modules/Jobs/Types/LogBuffer';

const chunk = (seq: number, text: string, extra: Partial<TJobLogChunk> = {}): TJobLogChunk => ({
  Seq: seq,
  Stream: 'stdout',
  StepIndex: 0,
  CreatedAt: '2026-11-07T10:00:00Z',
  Text: text,
  ...extra,
});

const bigBuffer = (count: number): ILogBuffer => {
  const chunks: TJobLogChunk[] = [];
  const per = 100;
  for (let seq = 1; seq <= count / per; seq += 1) {
    let text = '';
    for (let at = 0; at < per; at += 1) text += `line ${String((seq - 1) * per + at + 1)}\n`;
    chunks.push(chunk(seq, text));
  }
  return appendChunks(EMPTY_LOG, chunks);
};

const mount = (buffer: ILogBuffer, extra: Partial<ILogViewerProps> = {}) =>
  renderWithProviders(
    <LogViewer
      organizationId={ORG_ID}
      jobId={JOB_ID}
      attempt={1}
      stepNames={['install', 'test']}
      buffer={buffer}
      done
      hasOlder={false}
      loadingOlder={false}
      olderFailed={false}
      loadOlder={() => Promise.resolve()}
      {...extra}
    />,
  );

const rows = () => document.querySelectorAll('[data-line]');
const label = Ar['jobs.log.search.label'];

let restoreLayout: () => void = () => undefined;

beforeEach(() => {
  restoreLayout = installLayout();
  window.history.replaceState({}, '', '/jobs/x');
});

afterEach(() => {
  restoreLayout();
  window.history.replaceState({}, '', '/');
});

describe('the log viewer', () => {
  it('renders a small window of rows for a 50,000-line log', () => {
    mount(bigBuffer(50_000));
    expect(rows().length).toBeGreaterThan(0);
    expect(rows().length).toBeLessThan(300);
    expect(screen.getByRole('log')).toHaveAttribute('aria-live', 'off');
  });

  it('finds a line far outside the screen, scrolls to it and announces the count apart from the list', async () => {
    mount(bigBuffer(50_000));
    const input = screen.getByRole('searchbox', { name: label });
    fireEvent.change(input, { target: { value: 'line 30000' } });
    expect(await screen.findByText('line 30000')).toBeInTheDocument();
    const status = screen.getByRole('status', { name: '' });
    expect(status).toHaveTextContent(Ar['jobs.log.search.status'].replace('{n}', isolate('1')).replace('{total}', isolate('1')));
    expect(screen.getByRole('log')).not.toContainElement(status);
    expect(rows().length).toBeLessThan(300);
  });

  it('says when nothing matches and walks through results with the next and previous buttons', () => {
    mount(bigBuffer(1000));
    const input = screen.getByRole('searchbox', { name: label });
    fireEvent.change(input, { target: { value: 'zzz' } });
    expect(screen.getByText(Ar['jobs.log.search.empty'])).toBeInTheDocument();
    fireEvent.change(input, { target: { value: 'line 99' } });
    const next = screen.getByRole('button', { name: Ar['jobs.log.search.next'] });
    fireEvent.click(next);
    expect(screen.getByText(Ar['jobs.log.search.status'].replace('{n}', isolate('2')).replace('{total}', isolate('11')))).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: Ar['jobs.log.search.prev'] }));
    expect(screen.getByText(Ar['jobs.log.search.status'].replace('{n}', isolate('1')).replace('{total}', isolate('11')))).toBeInTheDocument();
  });

  it('focuses the search box with the slash key from inside the viewer', () => {
    mount(bigBuffer(100));
    screen.getByRole('log').focus();
    fireEvent.keyDown(screen.getByRole('log'), { key: '/' });
    expect(screen.getByRole('searchbox', { name: label })).toHaveFocus();
  });

});

describe('the log viewer links, steps and safety', () => {
  it('opens at the line of the link and marks it', async () => {
    window.history.replaceState({}, '', '/jobs/x?line=40000');
    mount(bigBuffer(50_000));
    const row = await waitFor(() => {
      const found = document.querySelector('[data-line="40000"]');
      expect(found).not.toBeNull();
      return found as HTMLElement;
    });
    expect(row).toHaveClass('bg-sel');
    expect(rows().length).toBeLessThan(300);
  });

  it('says so when the line of the link is not in the log', async () => {
    window.history.replaceState({}, '', '/jobs/x?line=900');
    mount(bigBuffer(100));
    expect(await screen.findByText(Ar['jobs.log.line.missing'].replace('{n}', isolate('900')))).toBeInTheDocument();
  });

  it('selects a line, puts it in the address and copies its link', async () => {
    mount(bigBuffer(100));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    const row = document.querySelector('[data-line]');
    const id = row?.getAttribute('data-line') ?? '';
    const button = row?.querySelector('button');
    expect(button).not.toBeNull();
    fireEvent.click(button as HTMLElement);
    expect(window.location.search).toBe(`?line=${id}&attempt=1`);
    expect(await screen.findByText(Ar['jobs.log.link.copied'])).toBeInTheDocument();
    expect(new URL(await navigator.clipboard.readText()).searchParams.get('line')).toBe(id);
  });

  it('points the download at the raw log of the attempt', () => {
    mount(bigBuffer(100));
    const link = screen.getByRole('link', { name: Ar['jobs.log.download'] });
    expect(link).toHaveAttribute('href', `/api/v1/organizations/${ORG_ID}/jobs/${JOB_ID}/logs/raw?Attempt=1`);
    expect(link).toHaveAttribute('download');
  });

  it('collapses and expands a step', () => {
    const buffer = appendChunks(EMPTY_LOG, [chunk(1, 'a\nb\n'), chunk(2, 'c\n', { StepIndex: 1 })]);
    mount(buffer);
    const header = screen.getByRole('button', { name: Ar['jobs.log.stepHeader'].replace('{n}', isolate('1')).replace('{name}', isolate('install')) });
    expect(header).toHaveAttribute('aria-expanded', 'true');
    fireEvent.click(header);
    expect(header).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('b')).not.toBeInTheDocument();
    expect(screen.getByText('c')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: Ar['jobs.log.collapse.all'] }));
    expect(screen.queryByText('c')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: Ar['jobs.log.collapse.none'] }));
    expect(screen.getByText('b')).toBeInTheDocument();
    expect(screen.getByText('c')).toBeInTheDocument();
  });

  it('offers to load older lines and keeps the search honest about them', () => {
    const loadOlder = vi.fn(() => Promise.resolve());
    mount(bigBuffer(100), { hasOlder: true, loadOlder });
    fireEvent.click(screen.getByRole('button', { name: Ar['jobs.log.older.load'] }));
    expect(loadOlder).toHaveBeenCalledOnce();
    fireEvent.change(screen.getByRole('searchbox', { name: label }), { target: { value: 'line' } });
    expect(screen.getByText(Ar['jobs.log.search.partial'])).toBeInTheDocument();
  });

  it('renders markup, bidi overrides and escape sequences as harmless text', () => {
    const hostile = '<img src=x onerror=alert(1)> ‮evil\u001b]8;;https://evil.example\u0007link\u001b]8;;\u0007 \u001b[2J';
    mount(appendChunks(EMPTY_LOG, [chunk(1, `${hostile}\n`)]));
    const box = screen.getByRole('log');
    expect(box.querySelector('img')).toBeNull();
    expect(box.querySelector('a')).toBeNull();
    expect(box).toHaveTextContent('<img src=x onerror=alert(1)>');
    expect(box).toHaveTextContent('U+202E');
    expect(box.textContent).not.toContain('‮');
    expect(box.textContent).not.toContain('evil.example');
  });
});

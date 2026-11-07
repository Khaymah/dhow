import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { FakeEventSource, installFakeEventSource } from '@/Core/Testing/FakeEventSource';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { JOB_ID } from '@/Core/Testing/JobsBackend';
import { renderWithProviders } from '@/Core/Testing/Render';
import { installLayout } from '@/Core/Testing/Layout';
import { Ar } from '@/I18n/Locales/Ar';
import { JobLog } from '@/Modules/Jobs/Components/JobLog';
import { FLUSH_MS } from '@/Modules/Jobs/Hooks/useJobLog';
import { isolate } from '@/I18n/Translate';

const chunk = (seq: number, text: string, extra: Record<string, unknown> = {}) => ({
  Seq: seq,
  Stream: 'stdout',
  StepIndex: 0,
  CreatedAt: '2026-10-28T10:00:00Z',
  Text: text,
  ...extra,
});

const mountLog = (onEnded = vi.fn()) => {
  const view = renderWithProviders(
    <JobLog organizationId={ORG_ID} jobId={JOB_ID} attempt={2} stepNames={['install', 'test']} onEnded={onEnded} />,
  );
  return { ...view, onEnded, source: FakeEventSource.latest() };
};

const deliver = async (source: FakeEventSource, events: [string, unknown][]) => {
  act(() => {
    source.open();
    for (const [type, data] of events) source.emit(type, data);
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, FLUSH_MS + 20));
  });
};

const log = () => screen.getByRole('log', { name: Ar['jobs.log.label'].replace('{n}', isolate('2')) });

let restoreLayout = () => undefined as void;

beforeEach(async () => {
  installFakeEventSource();
  restoreLayout = installLayout();
  await import('@/Modules/Jobs/Components/LogViewer');
});

afterEach(() => {
  restoreLayout();
});

describe('the job log', () => {
  it('opens one stream for the attempt and waits for output', () => {
    const { source } = mountLog();
    expect(source.url).toContain(`/api/v1/organizations/${ORG_ID}/jobs/${JOB_ID}/logs/stream?Attempt=2`);
    expect(source.url).not.toContain('FromSeq');
    expect(screen.getByText(Ar['jobs.log.waiting'])).toBeInTheDocument();
    expect(screen.getByText(Ar['jobs.log.connecting'])).toBeInTheDocument();
  });

  it('shows lines as they arrive, joins split lines and colors ANSI output with classes', async () => {
    const { source } = mountLog();
    await deliver(source, [
      ['status', { Attempt: 2, State: 'running', Truncated: false }],
      ['chunk', chunk(1, 'npm \u001b[31mERR!\u001b[0m bad')],
      ['chunk', chunk(2, ' thing\nsecond line\n')],
      ['chunk', chunk(3, 'oops\n', { Stream: 'stderr', StepIndex: 1 })],
    ]);
    expect(screen.getByText(Ar['jobs.log.live'])).toBeInTheDocument();
    expect(log()).toHaveTextContent('npm ERR! bad thing');
    expect(screen.getByText('ERR!')).toHaveClass('text-accent-coral');
    expect(screen.getByText('second line')).toBeInTheDocument();
    expect(screen.getByText(Ar['jobs.log.stepHeader'].replace('{n}', isolate('1')).replace('{name}', isolate('install')))).toBeInTheDocument();
    expect(screen.getByText(Ar['jobs.log.stepHeader'].replace('{n}', isolate('2')).replace('{name}', isolate('test')))).toBeInTheDocument();
    expect(screen.getByText('oops').closest('[data-stream]')).toHaveAttribute('data-stream', 'stderr');
    expect(document.querySelector('style')).toBeNull();
  });

  it('renders text as text, never as HTML', async () => {
    const { source } = mountLog();
    await deliver(source, [['chunk', chunk(1, '<img src=x onerror=alert(1)>\n')]]);
    expect(log().querySelector('img')).toBeNull();
    expect(log()).toHaveTextContent('<img src=x onerror=alert(1)>');
  });

  it('closes the stream at the end of the attempt and says the log is complete', async () => {
    const { source, onEnded } = mountLog();
    await deliver(source, [
      ['chunk', chunk(1, 'done\n')],
      ['end', { Attempt: 2, State: 'succeeded', Truncated: true }],
    ]);
    expect(source.readyState).toBe(FakeEventSource.CLOSED);
    expect(onEnded).toHaveBeenCalledOnce();
    expect(screen.getByText(Ar['jobs.log.ended'])).toBeInTheDocument();
    expect(screen.getByText(Ar['jobs.log.truncated'])).toBeInTheDocument();
  });

  it('says when a finished attempt wrote nothing', async () => {
    const { source } = mountLog();
    await deliver(source, [['end', { Attempt: 2, State: 'failed', Truncated: false }]]);
    expect(screen.getByText(Ar['jobs.log.empty'])).toBeInTheDocument();
  });

});

describe('the job log connection', () => {
  it('reconnects after the server closes the stream, from the line after the last one', async () => {
    const { source } = mountLog();
    await deliver(source, [['chunk', chunk(7, 'seven\n')]]);
    act(() => {
      source.fail(true);
    });
    expect(screen.getByText(Ar['jobs.log.reconnecting'])).toBeInTheDocument();
    await waitFor(
      () => {
        expect(FakeEventSource.instances).toHaveLength(2);
      },
      { timeout: 3000 },
    );
    expect(FakeEventSource.latest().url).toContain('FromSeq=8');
    await deliver(FakeEventSource.latest(), [['chunk', chunk(7, 'seven\n')], ['chunk', chunk(8, 'eight\n')]]);
    expect(screen.getAllByText('seven')).toHaveLength(1);
    expect(screen.getByText('eight')).toBeInTheDocument();
  });

  it('lets the browser resume on its own while it is still reconnecting', async () => {
    const { source } = mountLog();
    await deliver(source, [['chunk', chunk(1, 'one\n')]]);
    act(() => {
      source.fail(false);
    });
    expect(screen.getByText(Ar['jobs.log.reconnecting'])).toBeInTheDocument();
    expect(FakeEventSource.instances).toHaveLength(1);
  });

  it('gives up after repeated failures and offers a retry that resumes', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      mountLog();
      for (let round = 0; round <= 5; round += 1) {
        act(() => {
          FakeEventSource.latest().fail(true);
        });
        await act(async () => {
          await vi.advanceTimersByTimeAsync(16_000);
        });
      }
      expect(screen.getByText(Ar['jobs.log.failed'])).toBeInTheDocument();
      const before = FakeEventSource.instances.length;
      fireEvent.click(screen.getByRole('button', { name: Ar['jobs.retry'] }));
      expect(FakeEventSource.instances.length).toBe(before + 1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('stops following when the reader scrolls up and offers a jump back to the end', async () => {
    const { source, user } = mountLog();
    await deliver(source, [['chunk', chunk(1, 'a\nb\nc\n')]]);
    const box = log();
    Object.defineProperty(box, 'scrollHeight', { configurable: true, value: 1000 });
    Object.defineProperty(box, 'clientHeight', { configurable: true, value: 200 });
    box.scrollTop = 100;
    fireEvent.scroll(box);
    const jump = await screen.findByRole('button', { name: Ar['jobs.log.jump'] });
    await deliver(source, [['chunk', chunk(2, 'd\n')]]);
    expect(box.scrollTop).toBe(100);
    await user.click(jump);
    expect(box.scrollTop).toBe(1000);
    expect(screen.queryByRole('button', { name: Ar['jobs.log.jump'] })).not.toBeInTheDocument();
  });
});

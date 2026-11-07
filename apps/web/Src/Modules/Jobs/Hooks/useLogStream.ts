import { useEffect } from 'react';

import { jobLogStreamUrl, type TJobLogChunk, type TJobLogStatus } from '@/Modules/Jobs/Api/JobsApi';
import type { ILogSink } from '@/Modules/Jobs/Hooks/LogSink';

export const FLUSH_MS = 120;
export const MAX_FAILURES = 5;
const BACKOFF_MS = [1000, 2000, 4000, 8000, 15000];

const parse = (event: Event): unknown => {
  if (!(event instanceof MessageEvent) || typeof event.data !== 'string') return null;
  try {
    return JSON.parse(event.data) as unknown;
  } catch {
    return null;
  }
};

export const useLogStream = (sink: ILogSink, enabled: boolean, generation: number) => {
  const { organizationId, jobId, attempt } = sink.Target;
  const { LastSeq, Append, SetConnection, SetStatus, Ended } = sink;

  useEffect(() => {
    if (!enabled) return undefined;
    let source: EventSource | null = null;
    let pending: TJobLogChunk[] = [];
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let failures = 0;
    let stopped = false;

    const flush = () => {
      flushTimer = null;
      if (pending.length === 0) return;
      const batch = pending;
      pending = [];
      Append(batch);
    };

    const onChunk = (event: Event) => {
      const chunk = parse(event) as TJobLogChunk | null;
      if (chunk === null || chunk.Seq <= LastSeq.current) return;
      LastSeq.current = chunk.Seq;
      pending.push(chunk);
      flushTimer ??= setTimeout(flush, FLUSH_MS);
    };

    const onStatus = (event: Event) => {
      const status = parse(event) as TJobLogStatus | null;
      failures = 0;
      SetConnection('live');
      if (status !== null) SetStatus(status);
    };

    const onEnd = (event: Event) => {
      const status = parse(event) as TJobLogStatus | null;
      if (status !== null) SetStatus(status);
      source?.close();
      stopped = true;
      flush();
      SetConnection('ended');
      Ended();
    };

    const connect = () => {
      retryTimer = null;
      source = new EventSource(jobLogStreamUrl(organizationId, jobId, attempt, LastSeq.current + 1), { withCredentials: true });
      source.addEventListener('chunk', onChunk);
      source.addEventListener('status', onStatus);
      source.addEventListener('end', onEnd);
      source.onopen = () => {
        SetConnection('live');
      };
      source.onerror = () => {
        if (stopped || source === null) return;
        if (source.readyState !== EventSource.CLOSED) {
          SetConnection('reconnecting');
          return;
        }
        source.close();
        failures += 1;
        if (failures > MAX_FAILURES) {
          SetConnection('failed');
          return;
        }
        SetConnection('reconnecting');
        retryTimer = setTimeout(connect, BACKOFF_MS[Math.min(failures, BACKOFF_MS.length) - 1]);
      };
    };

    connect();
    return () => {
      stopped = true;
      source?.close();
      if (flushTimer !== null) clearTimeout(flushTimer);
      if (retryTimer !== null) clearTimeout(retryTimer);
      flush();
    };
  }, [enabled, generation, organizationId, jobId, attempt, LastSeq, Append, SetConnection, SetStatus, Ended]);
};

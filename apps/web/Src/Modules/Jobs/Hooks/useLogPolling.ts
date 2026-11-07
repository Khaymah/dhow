import { useEffect } from 'react';

import { getJobLogPage } from '@/Modules/Jobs/Api/JobsApi';
import type { ILogSink } from '@/Modules/Jobs/Hooks/LogSink';
import { isFinalState } from '@/Modules/Jobs/Types/JobStates';

export const POLL_MS = 2000;
export const POLL_MAX_FAILURES = 5;

export const useLogPolling = (sink: ILogSink, enabled: boolean, generation: number) => {
  const { organizationId, jobId, attempt } = sink.Target;
  const { LastSeq, Append, SetConnection, SetStatus, Ended } = sink;

  useEffect(() => {
    if (!enabled) return undefined;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let failures = 0;

    const schedule = (delay: number) => {
      timer = setTimeout(() => {
        void poll();
      }, delay);
    };

    const poll = async () => {
      timer = null;
      try {
        const page = await getJobLogPage(organizationId, jobId, attempt, LastSeq.current + 1);
        if (stopped) return;
        failures = 0;
        const fresh = page.Chunks.filter((chunk) => chunk.Seq > LastSeq.current);
        const last = fresh.at(-1);
        if (last !== undefined) {
          LastSeq.current = last.Seq;
          Append(fresh);
        }
        SetStatus({ Attempt: page.Attempt, State: page.State, Truncated: page.Truncated });
        SetConnection('polling');
        if (page.NextSeq !== undefined) {
          schedule(0);
          return;
        }
        if (isFinalState(page.State)) {
          SetConnection('ended');
          Ended();
          return;
        }
        schedule(POLL_MS);
      } catch {
        if (stopped) return;
        failures += 1;
        if (failures > POLL_MAX_FAILURES) {
          SetConnection('failed');
          return;
        }
        SetConnection('reconnecting');
        schedule(POLL_MS * failures);
      }
    };

    SetConnection('polling');
    void poll();
    return () => {
      stopped = true;
      if (timer !== null) clearTimeout(timer);
    };
  }, [enabled, generation, organizationId, jobId, attempt, LastSeq, Append, SetConnection, SetStatus, Ended]);
};

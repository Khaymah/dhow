import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { TJobLogChunk, TJobLogStatus } from '@/Modules/Jobs/Api/JobsApi';
import {
  type IJobLogTarget,
  type ILogSink,
  type TLogConnection,
  type TLogMode,
} from '@/Modules/Jobs/Hooks/LogSink';
import { fetchOlder } from '@/Modules/Jobs/Hooks/LogOlder';
import { useLogPolling } from '@/Modules/Jobs/Hooks/useLogPolling';
import { FLUSH_MS, MAX_FAILURES, useLogStream } from '@/Modules/Jobs/Hooks/useLogStream';
import { appendChunks, EMPTY_LOG, firstSeqOf, hasOlder, prependChunks, type ILogBuffer } from '@/Modules/Jobs/Types/LogBuffer';

export { FLUSH_MS, MAX_FAILURES };
export type { IJobLogTarget, TLogConnection, TLogMode };

export interface IJobLog {
  Buffer: ILogBuffer;
  Connection: TLogConnection;
  Mode: TLogMode;
  Status: TJobLogStatus | null;
  Retry: () => void;
  UsePolling: () => void;
  HasOlder: boolean;
  LoadingOlder: boolean;
  OlderFailed: boolean;
  LoadOlder: () => Promise<void>;
}

export const useJobLog = (target: IJobLogTarget): IJobLog => {
  const { organizationId, jobId, attempt, onEnded } = target;
  const [Buffer, setBuffer] = useState<ILogBuffer>(EMPTY_LOG);
  const [Connection, setConnection] = useState<TLogConnection>('connecting');
  const [Mode, setMode] = useState<TLogMode>('stream');
  const [Status, setStatus] = useState<TJobLogStatus | null>(null);
  const [Generation, setGeneration] = useState(0);
  const [LoadingOlder, setLoadingOlder] = useState(false);
  const [OlderFailed, setOlderFailed] = useState(false);
  const lastSeq = useRef(0);
  const endedRef = useRef(onEnded);
  const bufferRef = useRef(Buffer);
  const loadingRef = useRef(false);

  useEffect(() => {
    endedRef.current = onEnded;
  }, [onEnded]);

  useEffect(() => {
    bufferRef.current = Buffer;
  }, [Buffer]);

  const Append = useCallback((chunks: TJobLogChunk[]) => {
    setBuffer((buffer) => appendChunks(buffer, chunks));
  }, []);
  const Ended = useCallback(() => {
    endedRef.current?.();
  }, []);

  const sink = useMemo<ILogSink>(
    () => ({
      Target: { organizationId, jobId, attempt },
      LastSeq: lastSeq,
      Append,
      SetConnection: setConnection,
      SetStatus: setStatus,
      Ended,
    }),
    [organizationId, jobId, attempt, Append, Ended],
  );

  useLogStream(sink, Mode === 'stream', Generation);
  useLogPolling(sink, Mode === 'polling', Generation);

  const Retry = useCallback(() => {
    setConnection('connecting');
    setGeneration((value) => value + 1);
  }, []);

  const UsePolling = useCallback(() => {
    setMode('polling');
    setConnection('connecting');
  }, []);

  const LoadOlder = useCallback(async () => {
    const head = firstSeqOf(bufferRef.current);
    if (loadingRef.current || head === null || head <= 1) return;
    loadingRef.current = true;
    setLoadingOlder(true);
    setOlderFailed(false);
    try {
      const older = await fetchOlder(organizationId, jobId, attempt, head);
      setBuffer((buffer) => prependChunks(buffer, older));
    } catch {
      setOlderFailed(true);
    } finally {
      loadingRef.current = false;
      setLoadingOlder(false);
    }
  }, [organizationId, jobId, attempt]);

  return {
    Buffer,
    Connection,
    Mode,
    Status,
    Retry,
    UsePolling,
    HasOlder: hasOlder(Buffer),
    LoadingOlder,
    OlderFailed,
    LoadOlder,
  };
};

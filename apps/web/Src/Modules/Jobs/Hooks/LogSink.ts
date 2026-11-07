import type { RefObject } from 'react';

import type { TJobLogChunk, TJobLogStatus } from '@/Modules/Jobs/Api/JobsApi';

export type TLogConnection = 'connecting' | 'live' | 'reconnecting' | 'polling' | 'ended' | 'failed';

export type TLogMode = 'stream' | 'polling';

export interface IJobLogTarget {
  organizationId: string;
  jobId: string;
  attempt: number;
  onEnded?: () => void;
}

export interface ILogSink {
  Target: IJobLogTarget;
  LastSeq: RefObject<number>;
  Append: (chunks: TJobLogChunk[]) => void;
  SetConnection: (connection: TLogConnection) => void;
  SetStatus: (status: TJobLogStatus) => void;
  Ended: () => void;
}

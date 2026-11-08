import type { TBadgeTone } from '@/Core/Ui/Badge';
import type { TIconName } from '@/Core/Ui/Icons';
import type { TMessageKey } from '@/I18n/Messages';
import type { TBoardProgress, TBoardReleaseState, TBoardTarget } from '@/Modules/Boards/Api/BoardsApi';

export interface IStatusLook {
  Tone: TBadgeTone;
  Icon: TIconName;
  Label: TMessageKey;
}

const LOOKS: Record<TBoardReleaseState, IStatusLook> = {
  running: { Tone: 'info', Icon: 'play', Label: 'boards.status.running' },
  succeeded: { Tone: 'ok', Icon: 'check', Label: 'boards.status.succeeded' },
  failed: { Tone: 'bad', Icon: 'close', Label: 'boards.status.failed' },
  rolled_back: { Tone: 'warn', Icon: 'swap', Label: 'boards.status.rolled_back' },
  rollback_failed: { Tone: 'bad', Icon: 'alert', Label: 'boards.status.rollback_failed' },
  unknown: { Tone: 'warn', Icon: 'info', Label: 'boards.status.unknown' },
};

export const lookOf = (state: TBoardReleaseState): IStatusLook => LOOKS[state];

export const shortDigest = (digest: string | undefined): string => (digest === undefined ? '' : digest.slice(0, 12));

export const deployedLabelOf = (target: TBoardTarget): string => {
  const digest = shortDigest(target.CurrentDigest);
  return digest === '' ? (target.CurrentReleaseId ?? '') : digest;
};

const MAX_SEGMENTS = 12;

export type TSegment = 'done' | 'current' | 'waiting';

export const segmentsOf = (progress: TBoardProgress): TSegment[] | null => {
  if (progress.Batches > MAX_SEGMENTS) return null;
  const finished = Math.floor(progress.HostsDone / progress.BatchSize);
  return Array.from({ length: progress.Batches }, (_, index): TSegment => {
    if (index < finished) return 'done';
    return index === finished ? 'current' : 'waiting';
  });
};

export const currentBatchOf = (progress: TBoardProgress): number =>
  Math.min(Math.max(progress.Batch, Math.floor(progress.HostsDone / progress.BatchSize) + 1, 1), progress.Batches);

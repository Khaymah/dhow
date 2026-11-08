import { cx } from '@/Core/Ui/ClassNames';
import { useT } from '@/I18n/useT';
import type { TBoardProgress } from '@/Modules/Boards/Api/BoardsApi';
import { currentBatchOf, segmentsOf, type TSegment } from '@/Modules/Boards/Types/BoardStatus';

const SEGMENT: Record<TSegment, string> = {
  done: 'bg-accent-green',
  current: 'bg-accent-blue animate-dhow-pulse',
  waiting: 'bg-track',
};

export const BatchProgress = ({ progress }: { progress: TBoardProgress }) => {
  const t = useT();
  const segments = segmentsOf(progress);
  const started = progress.HostsDone > 0 || progress.Batch > 0;
  const batch = t('boards.progress.batch', { batch: currentBatchOf(progress), batches: progress.Batches });
  const hosts = t('boards.progress.hosts', { done: progress.HostsDone, hosts: progress.Hosts });
  const text = started ? `${batch} · ${hosts}` : t('boards.progress.starting', { hosts: progress.Hosts });

  return (
    <div
      role="progressbar"
      aria-label={t('boards.progress.label', { release: progress.ReleaseId })}
      aria-valuemin={0}
      aria-valuemax={progress.Hosts}
      aria-valuenow={progress.HostsDone}
      aria-valuetext={text}
      className="grid gap-1.5"
    >
      {segments === null ? null : (
        <span aria-hidden="true" className="flex gap-1">
          {segments.map((segment, index) => (
            <i key={index} data-segment={segment} className={cx('h-1.5 min-w-4 flex-1 rounded-pill', SEGMENT[segment])} />
          ))}
        </span>
      )}
      <small className="text-body-sm text-secondary">{text}</small>
    </div>
  );
};

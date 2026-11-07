import { Suspense } from 'react';

import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { lazyComponent } from '@/Core/Ui/LazyComponent';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { StatusDot, type TStatus } from '@/Core/Ui/StatusDot';
import { useT } from '@/I18n/useT';
import { useJobLog, type TLogConnection } from '@/Modules/Jobs/Hooks/useJobLog';

const LogViewer = lazyComponent(() => import('@/Modules/Jobs/Components/LogViewer').then((module) => module.LogViewer));

export interface IJobLogProps {
  organizationId: string;
  jobId: string;
  attempt: number;
  stepNames: readonly string[];
  onEnded?: () => void;
}

const CONNECTION_STATUS: Record<TLogConnection, TStatus> = {
  connecting: 'queued',
  live: 'building',
  reconnecting: 'awaiting',
  polling: 'building',
  ended: 'live',
  failed: 'failed',
};

const FALLBACK_STATES: readonly TLogConnection[] = ['connecting', 'reconnecting', 'failed'];

const ConnectionDot = ({ connection }: { connection: TLogConnection }) => {
  const t = useT();
  return (
    <StatusDot status={CONNECTION_STATUS[connection]} className="justify-self-end">
      {t(`jobs.log.${connection}`)}
    </StatusDot>
  );
};

export const JobLog = ({ organizationId, jobId, attempt, stepNames, onEnded }: IJobLogProps) => {
  const t = useT();
  const log = useJobLog({ organizationId, jobId, attempt, onEnded });
  const lines = log.Buffer.Lines;
  const done = log.Connection === 'ended';
  const canFallback = log.Mode === 'stream' && FALLBACK_STATES.includes(log.Connection);

  return (
    <div className="grid gap-2.5">
      {log.Connection === 'failed' ? null : (
        <div role="status" className="flex items-center justify-end">
          <ConnectionDot connection={log.Connection} />
        </div>
      )}
      {log.Connection === 'failed' ? (
        <div className="grid justify-items-start gap-2">
          <InlineAlert accent="failed" title={t('jobs.log.failed')}>
            {t('jobs.log.failedAbout')}
          </InlineAlert>
          <Button size="sm" onClick={log.Retry}>
            {t('jobs.retry')}
          </Button>
        </div>
      ) : null}
      {canFallback ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" onClick={log.UsePolling}>
            {t('jobs.log.fallback')}
          </Button>
          <p className="m-0 text-body-sm text-tertiary">{t('jobs.log.fallbackAbout')}</p>
        </div>
      ) : null}
      {log.Status?.Truncated ? <InlineAlert accent="awaiting" title={t('jobs.log.truncated')} /> : null}
      {lines.length === 0 ? (
        <div
          role="log"
          aria-live="off"
          aria-label={t('jobs.log.label', { n: attempt })}
          className="rounded-md border border-line bg-surface-inset px-3 py-2.5 font-sans text-body-sm text-tertiary"
        >
          {done ? t('jobs.log.empty') : t('jobs.log.waiting')}
        </div>
      ) : (
        <Suspense fallback={<Skeleton label={t('jobs.detail.loading')} lines={6} />}>
          <LogViewer
            organizationId={organizationId}
            jobId={jobId}
            attempt={attempt}
            stepNames={stepNames}
            buffer={log.Buffer}
            done={done}
            hasOlder={log.HasOlder}
            loadingOlder={log.LoadingOlder}
            olderFailed={log.OlderFailed}
            loadOlder={log.LoadOlder}
          />
        </Suspense>
      )}
    </div>
  );
};

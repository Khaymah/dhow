import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { MAX_LOG_LINES } from '@/Modules/Jobs/Types/LogBuffer';

export interface ILogViewerNoticesProps {
  hasOlder: boolean;
  loadingOlder: boolean;
  olderFailed: boolean;
  searching: boolean;
  missingLine: number | null;
  loadOlder: () => Promise<void>;
}

export const LogViewerNotices = ({ hasOlder, loadingOlder, olderFailed, searching, missingLine, loadOlder }: ILogViewerNoticesProps) => {
  const t = useT();
  return (
    <>
      {hasOlder ? (
        <div className="flex flex-wrap items-center gap-2">
          <p className="m-0 text-body-sm text-tertiary">{t('jobs.log.dropped', { count: MAX_LOG_LINES })}</p>
          <Button
            size="sm"
            disabled={loadingOlder}
            onClick={() => {
              void loadOlder();
            }}
          >
            {loadingOlder ? t('jobs.log.older.loading') : t('jobs.log.older.load')}
          </Button>
          {searching ? <span className="text-body-sm text-tertiary">{t('jobs.log.search.partial')}</span> : null}
        </div>
      ) : null}
      {olderFailed ? <InlineAlert accent="failed" title={t('jobs.log.older.failed')} /> : null}
      {missingLine === null ? null : <InlineAlert accent="awaiting" title={t('jobs.log.line.missing', { n: missingLine })} />}
    </>
  );
};

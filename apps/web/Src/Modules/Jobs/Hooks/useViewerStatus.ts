import { useT } from '@/I18n/useT';
import type { ILogSearch } from '@/Modules/Jobs/Hooks/useLogSearch';
import type { TNotice } from '@/Modules/Jobs/Hooks/useNotice';

export const useViewerStatus = (notice: TNotice, search: ILogSearch, newLines: number): string => {
  const t = useT();
  if (notice === 'copied') return t('jobs.log.link.copied');
  if (notice === 'failed') return t('jobs.log.link.failed');
  if (search.Query !== '') {
    return search.Matches.length === 0
      ? t('jobs.log.search.empty')
      : t('jobs.log.search.status', { n: search.Active + 1, total: search.Matches.length });
  }
  return newLines > 0 ? t('jobs.log.newLines', { count: newLines }) : '';
};

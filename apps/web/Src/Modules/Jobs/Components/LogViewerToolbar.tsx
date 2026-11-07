import type { KeyboardEvent, RefObject } from 'react';

import { Button } from '@/Core/Ui/Button';
import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import { Input } from '@/Core/Ui/Input';
import { useT } from '@/I18n/useT';
import type { ILogSearch } from '@/Modules/Jobs/Hooks/useLogSearch';

export interface ILogViewerToolbarProps {
  search: ILogSearch;
  searchRef: RefObject<HTMLInputElement | null>;
  hasSteps: boolean;
  allCollapsed: boolean;
  canCopy: boolean;
  downloadUrl: string;
  onToggleSteps: () => void;
  onCopyLink: () => void;
}

const handleSearchKey = (search: ILogSearch, event: KeyboardEvent<HTMLInputElement>) => {
  if (event.key === 'Enter') {
    event.preventDefault();
    search.Step(event.shiftKey ? -1 : 1);
  } else if (event.key === 'Escape') {
    event.preventDefault();
    search.Clear();
    event.currentTarget.blur();
  }
};

const LINK_CLASS = cx(
  'inline-flex h-btn-sm cursor-pointer items-center justify-center gap-2 rounded-btn border border-line-2 bg-transparent px-3',
  'text-label-sm whitespace-nowrap text-primary no-underline transition-colors duration-140 hover:bg-surface-raised max-md:min-h-11',
);

export const LogViewerToolbar = ({
  search,
  searchRef,
  hasSteps,
  allCollapsed,
  canCopy,
  downloadUrl,
  onToggleSteps,
  onCopyLink,
}: ILogViewerToolbarProps) => {
  const t = useT();
  const searching = search.Query !== '';
  return (
    <div role="toolbar" aria-label={t('jobs.log.toolbar')} className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-48 flex-1">
        <Input
          ref={searchRef}
          compact
          type="search"
          value={search.Query}
          placeholder={t('jobs.log.search.label')}
          aria-label={t('jobs.log.search.label')}
          onChange={(event) => {
            search.SetQuery(event.target.value);
          }}
          onKeyDown={(event) => {
            handleSearchKey(search, event);
          }}
          className="ps-9"
        />
        <span aria-hidden="true" className="pointer-events-none absolute inset-y-0 start-3 inline-grid place-items-center text-tertiary">
          <Icon name="search" size={16} />
        </span>
      </div>
      <Button size="sm" aria-pressed={search.MatchCase} variant={search.MatchCase ? 'primary' : 'secondary'} onClick={search.ToggleCase}>
        {t('jobs.log.search.case')}
      </Button>
      <Button size="sm" disabled={!searching || search.Matches.length === 0} onClick={() => { search.Step(-1); }}>
        {t('jobs.log.search.prev')}
      </Button>
      <Button size="sm" disabled={!searching || search.Matches.length === 0} onClick={() => { search.Step(1); }}>
        {t('jobs.log.search.next')}
      </Button>
      {hasSteps ? (
        <Button size="sm" onClick={onToggleSteps}>
          {allCollapsed ? t('jobs.log.collapse.none') : t('jobs.log.collapse.all')}
        </Button>
      ) : null}
      <Button size="sm" iconStart="copy" disabled={!canCopy} onClick={onCopyLink}>
        {t('jobs.log.link.copy')}
      </Button>
      <a href={downloadUrl} download className={LINK_CLASS}>
        <Icon name="download" size={16} />
        {t('jobs.log.download')}
      </a>
    </div>
  );
};

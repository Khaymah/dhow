import { useCallback, useId, useMemo, useRef, useState } from 'react';

import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';
import { jobLogRawUrl } from '@/Modules/Jobs/Api/JobsApi';
import { LogViewerList } from '@/Modules/Jobs/Components/LogViewerList';
import { LogViewerNotices } from '@/Modules/Jobs/Components/LogViewerNotices';
import { LogViewerToolbar } from '@/Modules/Jobs/Components/LogViewerToolbar';
import { useCollapsedSteps } from '@/Modules/Jobs/Hooks/useCollapsedSteps';
import { useLineTarget } from '@/Modules/Jobs/Hooks/useLineTarget';
import { useLogScroll } from '@/Modules/Jobs/Hooks/useLogScroll';
import { useLogSearch } from '@/Modules/Jobs/Hooks/useLogSearch';
import { useNotice } from '@/Modules/Jobs/Hooks/useNotice';
import { useSlashFocus } from '@/Modules/Jobs/Hooks/useSlashFocus';
import { useViewerStatus } from '@/Modules/Jobs/Hooks/useViewerStatus';
import type { ILogBuffer } from '@/Modules/Jobs/Types/LogBuffer';
import { lineUrl } from '@/Modules/Jobs/Types/LogLink';
import { visibleIndexes } from '@/Modules/Jobs/Types/LogRows';

export interface ILogViewerProps {
  organizationId: string;
  jobId: string;
  attempt: number;
  stepNames: readonly string[];
  buffer: ILogBuffer;
  done: boolean;
  hasOlder: boolean;
  loadingOlder: boolean;
  olderFailed: boolean;
  loadOlder: () => Promise<void>;
}

export const LogViewer = (props: ILogViewerProps) => {
  const { organizationId, jobId, attempt, stepNames, buffer, done, hasOlder, loadingOlder, olderFailed, loadOlder } = props;
  const t = useT();
  const lines = buffer.Lines;
  const hintId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const [Seen, setSeen] = useState(lines.length);
  const collapse = useCollapsedSteps(lines);
  const shown = useMemo(() => visibleIndexes(lines, collapse.Collapsed), [lines, collapse.Collapsed]);
  const { Virtualizer, Ref, Following, HandleScroll, JumpToEnd, ScrollTo } = useLogScroll(lines, shown);
  const target = useLineTarget({
    ...{ lines, shown, attempt, done, hasOlder, loadingOlder, olderFailed, loadOlder },
    expandStep: collapse.ExpandStep,
    scrollTo: ScrollTo,
  });
  const search = useLogSearch(lines, target.Goto);
  const { Notice, Copy } = useNotice();
  const matchSet = useMemo(() => new Set(search.Matches), [search.Matches]);
  useSlashFocus(rootRef, searchRef);

  if (Following && Seen !== lines.length) setSeen(lines.length);
  const status = useViewerStatus(Notice, search, Following ? 0 : Math.max(0, lines.length - Seen));
  const copyId = target.Selected ?? search.ActiveLine?.Id ?? null;

  const HandleCopy = useCallback(
    (id: number | null) => {
      if (id !== null) Copy(lineUrl(window.location.href, id, attempt));
    },
    [Copy, attempt],
  );
  const HandleSelect = useCallback(
    (id: number) => {
      target.Select(id);
      HandleCopy(id);
    },
    [target, HandleCopy],
  );

  return (
    <div ref={rootRef} className="grid gap-2.5">
      <LogViewerToolbar
        search={search}
        searchRef={searchRef}
        hasSteps={collapse.Steps.length > 0}
        allCollapsed={collapse.AllCollapsed}
        canCopy={copyId !== null}
        downloadUrl={jobLogRawUrl(organizationId, jobId, attempt)}
        onToggleSteps={collapse.ToggleAll}
        onCopyLink={() => {
          HandleCopy(copyId);
        }}
      />
      <p role="status" aria-live="polite" className="m-0 min-h-5 text-body-sm text-secondary">
        {status}
      </p>
      <LogViewerNotices
        {...{ hasOlder, loadingOlder, olderFailed, loadOlder }}
        searching={search.Query !== ''}
        missingLine={target.Missing}
      />
      <LogViewerList
        {...{ lines, shown, stepNames, attempt }}
        virtualizer={Virtualizer}
        scrollRef={Ref}
        onScroll={HandleScroll}
        collapsed={collapse.Collapsed}
        matches={matchSet}
        selectedId={target.Selected}
        currentId={search.ActiveLine?.Id ?? null}
        describedBy={hintId}
        onSelect={HandleSelect}
        onToggle={collapse.ToggleStep}
      />
      <p id={hintId} className="m-0 text-body-sm text-tertiary">
        {t('jobs.log.shortcuts')}
      </p>
      {!Following && lines.length > 0 ? (
        <Button size="sm" className="justify-self-start" onClick={JumpToEnd}>
          {t('jobs.log.jump')}
        </Button>
      ) : null}
    </div>
  );
};

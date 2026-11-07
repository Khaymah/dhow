import type { RefObject } from 'react';

import { useT } from '@/I18n/useT';
import { LogViewerLine } from '@/Modules/Jobs/Components/LogViewerLine';
import type { ILogScroll } from '@/Modules/Jobs/Hooks/useLogScroll';
import type { ILogLine } from '@/Modules/Jobs/Types/LogBuffer';

export interface ILogViewerListProps {
  virtualizer: ILogScroll['Virtualizer'];
  scrollRef: RefObject<HTMLDivElement | null>;
  onScroll: () => void;
  lines: readonly ILogLine[];
  shown: readonly number[];
  stepNames: readonly string[];
  attempt: number;
  collapsed: ReadonlySet<number>;
  matches: ReadonlySet<number>;
  selectedId: number | null;
  currentId: number | null;
  describedBy: string;
  onSelect: (id: number) => void;
  onToggle: (step: number) => void;
}

export const LogViewerList = ({
  virtualizer,
  scrollRef,
  onScroll,
  lines,
  shown,
  stepNames,
  attempt,
  collapsed,
  matches,
  selectedId,
  currentId,
  describedBy,
  onSelect,
  onToggle,
}: ILogViewerListProps) => {
  const t = useT();
  return (
    <div
      ref={scrollRef}
      role="log"
      aria-live="off"
      aria-label={t('jobs.log.label', { n: attempt })}
      aria-describedby={describedBy}
      /* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex */
      tabIndex={0}
      onScroll={onScroll}
      className="max-h-[min(36rem,70vh)] min-h-32 overflow-auto rounded-md border border-line bg-surface-inset px-3 py-2.5 font-mono text-code"
    >
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((item) => {
          const at = shown[item.index] ?? -1;
          const line = lines[at];
          if (line === undefined) return null;
          return (
            <div
              key={item.key}
              ref={virtualizer.measureElement}
              data-index={item.index}
              className="absolute inset-x-0 top-0"
              style={{ transform: `translateY(${String(item.start)}px)` }}
            >
              <LogViewerLine
                line={line}
                stepName={line.Step === undefined ? undefined : stepNames[line.Step]}
                collapsed={line.Step !== undefined && collapsed.has(line.Step)}
                selected={selectedId === line.Id}
                matched={matches.has(at)}
                current={currentId === line.Id}
                onSelect={onSelect}
                onToggle={onToggle}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
};

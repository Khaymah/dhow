import { memo } from 'react';

import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import { useT } from '@/I18n/useT';
import { systemCodeOf, type ILogLine } from '@/Modules/Jobs/Types/LogBuffer';

const SYSTEM_CODES = ['LOG_TRUNCATED', 'LATE_RESULT'] as const;
type TSystemCode = (typeof SYSTEM_CODES)[number];

const isSystemCode = (code: string | null): code is TSystemCode => SYSTEM_CODES.some((known) => known === code);

export interface ILogViewerLineProps {
  line: ILogLine;
  stepName: string | undefined;
  collapsed: boolean;
  selected: boolean;
  matched: boolean;
  current: boolean;
  onSelect: (id: number) => void;
  onToggle: (step: number) => void;
}

const StepHeader = ({ line, stepName, collapsed, onToggle }: Pick<ILogViewerLineProps, 'line' | 'stepName' | 'collapsed' | 'onToggle'>) => {
  const t = useT();
  const index = line.Step ?? 0;
  return (
    <button
      type="button"
      aria-expanded={!collapsed}
      onClick={() => {
        onToggle(index);
      }}
      className="flex w-full cursor-pointer items-center gap-2 border-0 border-t border-line bg-transparent pt-1.5 text-start font-sans text-label-sm text-secondary hover:text-primary"
    >
      <span aria-hidden="true" className={cx('inline-grid flex-none transition-transform duration-140', !collapsed && 'rotate-90 rtl:-rotate-90')}>
        <Icon name="next" size={14} />
      </span>
      {stepName === undefined ? t('jobs.log.stepHeaderUnnamed', { n: index + 1 }) : t('jobs.log.stepHeader', { n: index + 1, name: stepName })}
    </button>
  );
};

const LineBody = ({ line }: { line: ILogLine }) => {
  const t = useT();
  if (line.Kind === 'system') {
    const code = systemCodeOf(line.Raw);
    return (
      <div dir="auto" className="min-h-[1lh] min-w-0 flex-1 whitespace-pre-wrap break-words text-tertiary italic">
        {isSystemCode(code) ? t(`jobs.log.system.${code}`) : line.Segments.map((segment) => segment.Text).join('')}
      </div>
    );
  }
  return (
    <div
      dir="auto"
      data-stream={line.Stream}
      className={cx('min-h-[1lh] min-w-0 flex-1 whitespace-pre-wrap break-words', line.Stream === 'stderr' && 'border-s-2 border-bad-line ps-2')}
    >
      {line.Segments.map((segment, index) => (
        <span key={index} className={segment.ClassName === '' ? undefined : segment.ClassName}>
          {segment.Text}
        </span>
      ))}
    </div>
  );
};

export const LogViewerLine = memo(({ line, stepName, collapsed, selected, matched, current, onSelect, onToggle }: ILogViewerLineProps) => {
  const t = useT();
  if (line.Kind === 'step') return <StepHeader line={line} stepName={stepName} collapsed={collapsed} onToggle={onToggle} />;
  return (
    <div
      data-line={line.Id}
      className={cx('flex items-start gap-2', matched && 'bg-hover', current && 'bg-warn-wash', selected && 'bg-sel')}
    >
      <button
        type="button"
        tabIndex={-1}
        aria-label={t('jobs.log.line.select', { n: line.Id })}
        onClick={() => {
          onSelect(line.Id);
        }}
        className="w-14 flex-none cursor-pointer select-none border-0 bg-transparent px-0 text-end text-tertiary tabular-nums hover:text-primary"
      >
        {line.Id}
      </button>
      <LineBody line={line} />
    </div>
  );
});

LogViewerLine.displayName = 'LogViewerLine';

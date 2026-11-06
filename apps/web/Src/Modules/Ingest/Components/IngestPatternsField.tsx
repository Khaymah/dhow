import { Button } from '@/Core/Ui/Button';
import { IconButton } from '@/Core/Ui/IconButton';
import { Input } from '@/Core/Ui/Input';
import { Select } from '@/Core/Ui/Select';
import { useT } from '@/I18n/useT';
import type { TIngestRefKind } from '@/Modules/Ingest/Api/IngestApi';
import { MAX_PATTERNS, PATTERN_EXAMPLES, PATTERN_MAX, type IPatternDraft } from '@/Modules/Ingest/Types/IngestForm';

export interface IIngestPatternsFieldProps {
  patterns: IPatternDraft[];
  disabled: boolean;
  onChange: (patterns: IPatternDraft[]) => void;
}

const KINDS: readonly TIngestRefKind[] = ['branch', 'tag'];

export const IngestPatternsField = ({ patterns, disabled, onChange }: IIngestPatternsFieldProps) => {
  const t = useT();
  const replace = (index: number, next: IPatternDraft) => {
    onChange(patterns.map((item, position) => (position === index ? next : item)));
  };

  return (
    <fieldset className="m-0 grid gap-2 border-0 p-0" disabled={disabled}>
      <legend className="p-0 text-body-sm text-secondary">{t('ingest.patterns.title')}</legend>
      <small className="text-body-sm text-tertiary">{t('ingest.patterns.about')}</small>
      <small className="flex flex-wrap items-center gap-2 text-body-sm text-tertiary">
        {t('ingest.patterns.examples')}
        {PATTERN_EXAMPLES.map((example) => (
          <bdi key={example} dir="ltr" className="font-mono">
            {example}
          </bdi>
        ))}
      </small>
      {patterns.map((item, index) => (
        <div key={`${String(index)}-${item.Kind}`} className="flex min-w-0 items-center gap-2">
          <div className="w-28 flex-none">
            <Select
              compact
              aria-label={t('ingest.patterns.kind')}
              value={item.Kind}
              options={KINDS.map((kind) => ({ value: kind, label: t(`ingest.kind.${kind}`) }))}
              onChange={(event) => {
                replace(index, { ...item, Kind: event.target.value === 'tag' ? 'tag' : 'branch' });
              }}
            />
          </div>
          <div className="min-w-0 flex-1">
            <Input
              compact
              dir="ltr"
              className="font-mono"
              aria-label={t('ingest.patterns.pattern')}
              maxLength={PATTERN_MAX}
              autoComplete="off"
              spellCheck={false}
              value={item.Pattern}
              onChange={(event) => {
                replace(index, { ...item, Pattern: event.target.value });
              }}
            />
          </div>
          <IconButton
            size="sm"
            className="flex-none"
            icon="trash"
            label={t('ingest.patterns.remove')}
            disabled={patterns.length <= 1}
            onClick={() => {
              onChange(patterns.filter((_, position) => position !== index));
            }}
          />
        </div>
      ))}
      <Button
        size="sm"
        iconStart="plus"
        className="justify-self-start"
        disabled={patterns.length >= MAX_PATTERNS}
        onClick={() => {
          onChange([...patterns, { Kind: 'branch', Pattern: '' }]);
        }}
      >
        {t('ingest.patterns.add')}
      </Button>
    </fieldset>
  );
};

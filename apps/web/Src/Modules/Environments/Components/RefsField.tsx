import { Button } from '@/Core/Ui/Button';
import { IconButton } from '@/Core/Ui/IconButton';
import { Input } from '@/Core/Ui/Input';
import { Select } from '@/Core/Ui/Select';
import { useT } from '@/I18n/useT';
import type { TRefKind } from '@/Modules/Environments/Api/EnvironmentsApi';
import { MAX_REFS, PATTERN_MAX } from '@/Modules/Environments/Types/EnvironmentRules';
import type { IRefDraft } from '@/Modules/Environments/Types/RulesDraft';

export interface IRefsFieldProps {
  refs: IRefDraft[];
  disabled: boolean;
  fallbackHint: boolean;
  onChange: (refs: IRefDraft[]) => void;
}

const KINDS: readonly TRefKind[] = ['branch', 'tag'];

export const RefsField = ({ refs, disabled, fallbackHint, onChange }: IRefsFieldProps) => {
  const t = useT();
  const replace = (index: number, next: IRefDraft) => {
    onChange(refs.map((ref, position) => (position === index ? next : ref)));
  };

  return (
    <fieldset className="m-0 grid gap-2 border-0 p-0" disabled={disabled}>
      <legend className="p-0 text-body-sm text-secondary">{t('environments.refs.title')}</legend>
      <small className="text-body-sm text-tertiary">{t('environments.refs.about')}</small>
      {refs.length === 0 ? (
        <p className="m-0 text-body-sm text-secondary">{fallbackHint ? t('environments.refs.default') : t('environments.refs.none')}</p>
      ) : null}
      {refs.map((ref, index) => (
        <div key={`${String(index)}-${ref.Kind}`} className="flex min-w-0 items-center gap-2">
          <Select
            compact
            aria-label={t('environments.refs.kind')}
            className="w-32 flex-none"
            value={ref.Kind}
            options={KINDS.map((kind) => ({ value: kind, label: t(`environments.refs.kind.${kind}`) }))}
            onChange={(event) => {
              replace(index, { ...ref, Kind: event.target.value === 'tag' ? 'tag' : 'branch' });
            }}
          />
          <Input
            compact
            dir="ltr"
            className="font-mono"
            aria-label={t('environments.refs.pattern')}
            maxLength={PATTERN_MAX}
            autoComplete="off"
            spellCheck={false}
            value={ref.Pattern}
            onChange={(event) => {
              replace(index, { ...ref, Pattern: event.target.value });
            }}
          />
          <IconButton
            size="sm"
            icon="trash"
            label={t('environments.refs.remove')}
            onClick={() => {
              onChange(refs.filter((_, position) => position !== index));
            }}
          />
        </div>
      ))}
      <Button
        size="sm"
        iconStart="plus"
        className="justify-self-start"
        disabled={refs.length >= MAX_REFS}
        onClick={() => {
          onChange([...refs, { Kind: 'branch', Pattern: '' }]);
        }}
      >
        {t('environments.refs.add')}
      </Button>
    </fieldset>
  );
};

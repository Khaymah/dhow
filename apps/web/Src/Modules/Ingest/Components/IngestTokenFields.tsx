import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';
import { useT } from '@/I18n/useT';
import { IngestPatternsField } from '@/Modules/Ingest/Components/IngestPatternsField';
import { MAX_DAYS, TOKEN_NAME_MAX, type ITokenDraft } from '@/Modules/Ingest/Types/IngestForm';

export interface IIngestTokenFieldsProps {
  draft: ITokenDraft;
  disabled: boolean;
  onChange: (draft: ITokenDraft) => void;
}

export const IngestTokenFields = ({ draft, disabled, onChange }: IIngestTokenFieldsProps) => {
  const t = useT();

  return (
    <>
      <div className="grid gap-1.5">
        <Field label={t('ingest.form.name')}>
          <Input
            maxLength={TOKEN_NAME_MAX}
            autoComplete="off"
            disabled={disabled}
            value={draft.Name}
            onChange={(event) => {
              onChange({ ...draft, Name: event.target.value });
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('ingest.form.name.hint')}</small>
      </div>
      <IngestPatternsField
        patterns={draft.Patterns}
        disabled={disabled}
        onChange={(Patterns) => {
          onChange({ ...draft, Patterns });
        }}
      />
      <div className="grid gap-1.5">
        <Field label={t('ingest.form.days')}>
          <Input
            type="number"
            inputMode="numeric"
            min={1}
            max={MAX_DAYS}
            dir="ltr"
            disabled={disabled}
            value={draft.Days}
            onChange={(event) => {
              onChange({ ...draft, Days: event.target.value });
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('ingest.form.days.hint')}</small>
      </div>
    </>
  );
};

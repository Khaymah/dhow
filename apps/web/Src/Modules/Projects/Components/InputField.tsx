import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';
import { Select } from '@/Core/Ui/Select';
import { useT } from '@/I18n/useT';
import type { TManualInput } from '@/Modules/Projects/Api/RunsApi';
import type { TInputValue } from '@/Modules/Projects/Types/TriggerForm';

export interface IInputFieldProps {
  input: TManualInput;
  value: TInputValue | undefined;
  error?: string;
  disabled: boolean;
  onChange: (value: TInputValue) => void;
}

export const InputField = ({ input, value, error, disabled, onChange }: IInputFieldProps) => {
  const t = useT();
  const label = input.Required ? t('runs.trigger.input.required', { name: input.Name }) : input.Name;
  const about = input.Description === undefined || input.Description === '' ? null : (
    <small className="text-body-sm text-tertiary">
      <bdi>{input.Description}</bdi>
    </small>
  );

  if (input.Type === 'boolean') {
    return (
      <div className="grid gap-1">
        <label className="flex items-center gap-2.5 text-body-sm text-primary">
          <input type="checkbox" className="size-6 flex-none accent-accent" checked={value === true} disabled={disabled} onChange={(event) => { onChange(event.target.checked); }} />
          <bdi dir="ltr" className="font-mono">
            {input.Name}
          </bdi>
        </label>
        {about}
      </div>
    );
  }
  if (input.Type === 'choice') {
    const options = [...(input.Required ? [] : [{ value: '', label: t('runs.trigger.input.none') }]), ...input.Options.map((option) => ({ value: option, label: option }))];
    return (
      <div className="grid gap-1">
        <Field label={label} error={error}>
          <Select options={options} value={typeof value === 'string' ? value : ''} disabled={disabled} onChange={(event) => { onChange(event.target.value); }} />
        </Field>
        {about}
      </div>
    );
  }
  return (
    <div className="grid gap-1">
      <Field label={label} error={error}>
        <Input dir="auto" autoComplete="off" spellCheck={false} value={typeof value === 'string' ? value : ''} disabled={disabled} onChange={(event) => { onChange(event.target.value); }} />
      </Field>
      {about}
    </div>
  );
};

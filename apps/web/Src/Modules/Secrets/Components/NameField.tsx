import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';

export interface INameFieldProps {
  label: string;
  hint: string;
  value: string;
  onChange: (value: string) => void;
}

export const NameField = ({ label, hint, value, onChange }: INameFieldProps) => (
  <div className="grid gap-1.5">
    <Field label={label}>
      <Input
        dir="ltr"
        className="font-mono"
        maxLength={64}
        autoComplete="off"
        spellCheck={false}
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
        }}
      />
    </Field>
    <small className="text-body-sm text-tertiary">{hint}</small>
  </div>
);

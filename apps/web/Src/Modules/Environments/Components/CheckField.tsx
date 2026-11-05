import { useId } from 'react';

export interface ICheckFieldProps {
  label: string;
  about: string;
  checked: boolean;
  disabled: boolean;
  onChange: (checked: boolean) => void;
}

export const CheckField = ({ label, about, checked, disabled, onChange }: ICheckFieldProps) => {
  const id = useId();
  return (
    <div className="flex items-start gap-3">
      <input
        id={id}
        type="checkbox"
        aria-describedby={`${id}-about`}
        className="mt-1 size-4 flex-none cursor-pointer accent-accent"
        checked={checked}
        disabled={disabled}
        onChange={(event) => {
          onChange(event.target.checked);
        }}
      />
      <div className="grid gap-px">
        <label htmlFor={id} className="cursor-pointer text-label text-primary">
          {label}
        </label>
        <small id={`${id}-about`} className="text-body-sm text-tertiary">
          {about}
        </small>
      </div>
    </div>
  );
};

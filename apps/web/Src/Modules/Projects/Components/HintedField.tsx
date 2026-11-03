import type { ReactNode } from 'react';

import { Field } from '@/Core/Ui/Field';

export interface IHintedFieldProps {
  label: string;
  hint: string;
  error?: string;
  children: ReactNode;
}

export const HintedField = ({ label, hint, error, children }: IHintedFieldProps) => (
  <div className="grid gap-1.5">
    <Field label={label} error={error}>
      {children}
    </Field>
    <small className="text-body-sm text-tertiary">{hint}</small>
  </div>
);

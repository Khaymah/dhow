import { useContext, type ComponentProps } from 'react';

import { cx } from '@/Core/Ui/ClassNames';
import { FieldContext } from '@/Core/Ui/FieldContext';

export interface ITextareaProps extends ComponentProps<'textarea'> {
  invalid?: boolean;
}

export const Textarea = ({ invalid, className, rows = 4, ...rest }: ITextareaProps) => {
  const field = useContext(FieldContext);
  const bad = invalid ?? field?.invalid ?? false;

  return (
    <textarea
      id={field?.id}
      rows={rows}
      aria-invalid={bad ? true : undefined}
      aria-describedby={field?.errorId}
      className={cx(
        'w-full min-w-0 resize-y rounded-input border border-line-2 bg-input px-3.5 py-2.5 text-body text-primary placeholder:text-tertiary',
        'aria-invalid:border-accent-coral',
        className,
      )}
      {...rest}
    />
  );
};

import { useId } from 'react';

import { useT } from '@/I18n/useT';

export interface IPullRequestsToggleProps {
  checked: boolean;
  disabled: boolean;
  onChange: (checked: boolean) => void;
}

export const PullRequestsToggle = ({ checked, disabled, onChange }: IPullRequestsToggleProps) => {
  const t = useT();
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
          {t('secrets.form.pullRequests')}
        </label>
        <small id={`${id}-about`} className="text-body-sm text-tertiary">
          {t('secrets.form.pullRequests.about')}
        </small>
      </div>
    </div>
  );
};

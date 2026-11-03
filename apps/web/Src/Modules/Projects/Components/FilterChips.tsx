import { cx } from '@/Core/Ui/ClassNames';

export interface IFilterChip<T extends string> {
  id: T;
  label: string;
}

export interface IFilterChipsProps<T extends string> {
  label: string;
  chips: readonly IFilterChip<T>[];
  value: T;
  disabled?: boolean;
  onChange: (id: T) => void;
}

export const FilterChips = <T extends string>({ label, chips, value, disabled = false, onChange }: IFilterChipsProps<T>) => (
  <div className="min-w-0">
    <div role="group" aria-label={label} className="flex flex-wrap gap-1.5 pb-1">
      {chips.map((chip) => {
        const pressed = chip.id === value;
        return (
          <button
            key={chip.id}
            type="button"
            aria-pressed={pressed}
            disabled={disabled}
            onClick={() => {
              onChange(chip.id);
            }}
            className={cx(
              'h-btn-sm flex-none cursor-pointer rounded-pill border px-3 text-label-sm whitespace-nowrap transition-colors duration-140 max-md:h-11',
              pressed ? 'border-line-2 bg-surface-raised text-primary' : 'border-line bg-transparent text-secondary hover:text-primary',
            )}
          >
            {chip.label}
          </button>
        );
      })}
    </div>
  </div>
);

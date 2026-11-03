import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import { useT } from '@/I18n/useT';

export type TLinkStep = 'source' | 'deploy-key' | 'host-key' | 'webhook' | 'status';

export const stepsFor = (ssh: boolean): readonly TLinkStep[] =>
  ssh ? ['source', 'deploy-key', 'host-key', 'webhook', 'status'] : ['source', 'webhook', 'status'];

export interface ILinkStepperProps {
  steps: readonly TLinkStep[];
  current: TLinkStep;
}

export const LinkStepper = ({ steps, current }: ILinkStepperProps) => {
  const t = useT();
  const at = steps.indexOf(current);
  return (
    <ol aria-label={t('repository.steps.label')} className="m-0 flex list-none flex-wrap gap-x-4 gap-y-2 p-0">
      {steps.map((step, index) => (
        <li
          key={step}
          aria-current={step === current ? 'step' : undefined}
          className={cx('inline-flex items-center gap-1.5 text-label-sm', step === current ? 'text-primary' : 'text-tertiary')}
        >
          {index < at ? (
            <Icon name="check" size={15} className="text-ok-fg" />
          ) : (
            <span aria-hidden="true" className={cx('inline-grid size-5 place-items-center rounded-full border text-caption', step === current ? 'border-accent text-accent' : 'border-line-2')}>
              {index + 1}
            </span>
          )}
          {t(`repository.step.${step}`)}
        </li>
      ))}
    </ol>
  );
};

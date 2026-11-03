import { ChoiceList } from '@/Core/Ui/ChoiceList';
import { useT } from '@/I18n/useT';
import type { TForkPolicy } from '@/Modules/Projects/Api/ProjectsApi';

export interface IForkPolicyChoiceProps {
  value: TForkPolicy;
  allowAutomatic: boolean;
  disabled?: boolean;
  onChange: (value: TForkPolicy) => void;
}

export const ForkPolicyChoice = ({ value, allowAutomatic, disabled = false, onChange }: IForkPolicyChoiceProps) => {
  const t = useT();
  return (
    <div className="grid gap-1.5">
      <span className="text-body-sm text-secondary">{t('projects.fork.title')}</span>
      <ChoiceList
        label={t('projects.fork.title')}
        value={value}
        onChange={onChange}
        disabled={disabled}
        choices={[
          { value: 'approval', title: t('projects.fork.approval'), about: t('projects.fork.approval.about') },
          {
            value: 'automatic',
            title: t('projects.fork.automatic'),
            about: allowAutomatic ? t('projects.fork.automatic.about') : t('projects.fork.automatic.adminOnly'),
            disabled: !allowAutomatic,
          },
        ]}
      />
    </div>
  );
};

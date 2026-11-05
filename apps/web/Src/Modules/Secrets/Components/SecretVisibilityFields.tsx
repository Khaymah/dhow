import { ChoiceList } from '@/Core/Ui/ChoiceList';
import { CheckList } from '@/Core/Ui/CheckList';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TSecretVisibility } from '@/Modules/Secrets/Api/SecretsApi';
import { useProjectChoices } from '@/Modules/Secrets/Hooks/useSecretsData';

export interface ISecretVisibilityFieldsProps {
  organizationId: string;
  visibility: TSecretVisibility;
  projectIds: string[];
  disabled: boolean;
  onVisibility: (visibility: TSecretVisibility) => void;
  onProjectIds: (ids: string[]) => void;
}

const ProjectChoices = ({
  organizationId,
  projectIds,
  disabled,
  onProjectIds,
}: Pick<ISecretVisibilityFieldsProps, 'organizationId' | 'projectIds' | 'disabled' | 'onProjectIds'>) => {
  const t = useT();
  const projects = useProjectChoices(organizationId);
  if (projects.isError) {
    return (
      <div className="grid justify-items-start gap-2">
        <InlineAlert accent="failed" title={t('secrets.form.projects.error')} />
        <Button
          size="sm"
          onClick={() => {
            void projects.refetch();
          }}
        >
          {t('status.retry')}
        </Button>
      </div>
    );
  }
  if (projects.data === undefined) return <Skeleton label={t('secrets.form.projects.loading')} lines={3} />;
  if (projects.data.length === 0) return <p className="m-0 text-body-sm text-secondary">{t('secrets.form.projects.empty')}</p>;
  return (
    <CheckList
      label={t('secrets.form.projects')}
      items={projects.data.map((project) => ({
        value: project.Id,
        title: project.Name,
        about: project.Slug,
      }))}
      values={projectIds}
      onChange={onProjectIds}
      disabled={disabled}
    />
  );
};

export const SecretVisibilityFields = (props: ISecretVisibilityFieldsProps) => {
  const t = useT();
  return (
    <div className="grid gap-2.5">
      <ChoiceList<TSecretVisibility>
        label={t('secrets.form.visibility')}
        value={props.visibility}
        onChange={props.onVisibility}
        disabled={props.disabled}
        choices={[
          {
            value: 'all_projects',
            title: t('secrets.form.visibility.all'),
            about: t('secrets.form.visibility.all.about'),
          },
          {
            value: 'selected_projects',
            title: t('secrets.form.visibility.selected'),
            about: t('secrets.form.visibility.selected.about'),
          },
        ]}
      />
      {props.visibility === 'selected_projects' ? (
        <ProjectChoices
          organizationId={props.organizationId}
          projectIds={props.projectIds}
          disabled={props.disabled}
          onProjectIds={props.onProjectIds}
        />
      ) : null}
    </div>
  );
};

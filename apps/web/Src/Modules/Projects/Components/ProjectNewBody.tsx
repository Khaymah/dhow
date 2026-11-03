import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { createProject, projectKey, projectListKey, type TForkPolicy } from '@/Modules/Projects/Api/ProjectsApi';
import { ForkPolicyChoice } from '@/Modules/Projects/Components/ForkPolicyChoice';
import { HintedField } from '@/Modules/Projects/Components/HintedField';
import { useIssueText, type TIssues } from '@/Modules/Projects/Hooks/useIssueText';
import { useOrganizationAccess } from '@/Modules/Projects/Hooks/useProjectAccess';
import { canCreateProjects, canRunForksWithoutApproval } from '@/Modules/Projects/Types/ProjectAccess';
import {
  DEFAULT_PIPELINE_PATH,
  PIPELINE_EXTENSIONS,
  PIPELINE_PATH_MAX,
  PROJECT_NAME_MAX,
  PROJECT_SLUG_MAX,
  projectFormSchema,
  slugOf,
} from '@/Modules/Projects/Types/ProjectForm';

export interface IProjectNewBodyProps {
  onCreated: (projectId: string) => void;
}

interface IFormProps extends IProjectNewBodyProps {
  organizationId: string;
  allowAutomatic: boolean;
}

const ProjectForm = ({ organizationId, allowAutomatic, onCreated }: IFormProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const issueText = useIssueText();
  const client = useQueryClient();
  const [Name, setName] = useState('');
  const [Slug, setSlug] = useState<string | null>(null);
  const [PipelinePath, setPipelinePath] = useState(DEFAULT_PIPELINE_PATH);
  const [ForkPolicy, setForkPolicy] = useState<TForkPolicy>('approval');
  const [Problems, setProblems] = useState<TIssues>({});
  const [ServerError, setServerError] = useState<string | null>(null);
  const [Busy, setBusy] = useState(false);
  const slug = Slug ?? slugOf(Name);

  const HandleSubmit = () => {
    const parsed = projectFormSchema.safeParse({ Name, Slug: slug, PipelinePath });
    if (!parsed.success) {
      setProblems(issueText(parsed.error));
      return;
    }
    setProblems({});
    setServerError(null);
    setBusy(true);
    createProject(organizationId, { ...parsed.data, ForkPolicy })
      .then((project) => {
        client.setQueryData(projectKey(organizationId, project.Id), project);
        void client.invalidateQueries({ queryKey: projectListKey(organizationId) });
        onCreated(project.Id);
      })
      .catch((error: unknown) => {
        setServerError(errorText(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <form
      noValidate
      className="grid gap-5"
      onSubmit={(event) => {
        event.preventDefault();
        HandleSubmit();
      }}
    >
      <p className="m-0 text-body-sm text-secondary">{t('projects.new.lead')}</p>
      {ServerError === null ? null : <InlineAlert accent="failed" title={ServerError} />}
      <HintedField label={t('projects.form.name')} hint={t('projects.form.name.hint')} error={Problems.Name}>
        <Input
          maxLength={PROJECT_NAME_MAX}
          autoComplete="off"
          value={Name}
          disabled={Busy}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </HintedField>
      <HintedField label={t('projects.form.slug')} hint={t('projects.form.slug.hint')} error={Problems.Slug}>
        <Input
          dir="ltr"
          maxLength={PROJECT_SLUG_MAX}
          autoComplete="off"
          spellCheck={false}
          value={slug}
          disabled={Busy}
          onChange={(event) => {
            setSlug(event.target.value);
          }}
        />
      </HintedField>
      <HintedField label={t('projects.form.path')} hint={t('projects.form.path.hint', { ext: PIPELINE_EXTENSIONS, example: DEFAULT_PIPELINE_PATH })} error={Problems.PipelinePath}>
        <Input
          dir="ltr"
          maxLength={PIPELINE_PATH_MAX}
          autoComplete="off"
          spellCheck={false}
          value={PipelinePath}
          disabled={Busy}
          onChange={(event) => {
            setPipelinePath(event.target.value);
          }}
        />
      </HintedField>
      <ForkPolicyChoice value={ForkPolicy} allowAutomatic={allowAutomatic} disabled={Busy} onChange={setForkPolicy} />
      <Button type="submit" variant="primary" iconStart="plus" className="justify-self-start" disabled={Busy}>
        {Busy ? t('projects.new.submitting') : t('projects.new.submit')}
      </Button>
    </form>
  );
};

export const ProjectNewBody = ({ onCreated }: IProjectNewBodyProps) => {
  const t = useT();
  const access = useOrganizationAccess();
  if (access.Kind === 'loading') return <Skeleton label={t('ui.loading')} lines={5} />;
  if (access.Kind === 'denied' || !canCreateProjects(access.Role)) {
    return (
      <Card>
        <EmptyState icon="shield" text={t('projects.new.forbidden')} />
      </Card>
    );
  }
  return (
    <ProjectForm organizationId={access.OrganizationId} allowAutomatic={canRunForksWithoutApproval(access.Role)} onCreated={onCreated} />
  );
};

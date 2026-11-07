import { useState } from 'react';

import { z } from '@/Core/Forms/Zod';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { useT } from '@/I18n/useT';
import type { TForkPolicy, TProject, TUpdateProject } from '@/Modules/Projects/Api/ProjectsApi';
import { ForkPolicyChoice } from '@/Modules/Projects/Components/ForkPolicyChoice';
import { HintedField } from '@/Modules/Projects/Components/HintedField';
import { useIssueText, type TIssues } from '@/Modules/Projects/Hooks/useIssueText';
import { useArchiveProject, useUpdateProject } from '@/Modules/Projects/Hooks/useProjects';
import {
  DEFAULT_PIPELINE_PATH,
  nameField,
  PIPELINE_EXTENSIONS,
  PIPELINE_PATH_MAX,
  PROJECT_NAME_MAX,
  pipelinePathField,
} from '@/Modules/Projects/Types/ProjectForm';

export interface IProjectSettingsCardProps {
  organizationId: string;
  project: TProject;
  manager: boolean;
  allowAutomatic: boolean;
}

const settingsSchema = z.object({ Name: nameField, PipelinePath: pipelinePathField });

const changesOf = (project: TProject, next: { Name: string; PipelinePath: string; ForkPolicy: TForkPolicy }): TUpdateProject => ({
  ...(next.Name === project.Name ? {} : { Name: next.Name }),
  ...(next.PipelinePath === project.PipelinePath ? {} : { PipelinePath: next.PipelinePath }),
  ...(next.ForkPolicy === project.ForkPolicy ? {} : { ForkPolicy: next.ForkPolicy }),
});

export const ProjectSettingsCard = ({ organizationId, project, manager, allowAutomatic }: IProjectSettingsCardProps) => {
  const t = useT();
  const issueText = useIssueText();
  const Push = useToastStore((state) => state.Push);
  const save = useUpdateProject(organizationId, project.Id);
  const [Name, setName] = useState(project.Name);
  const [PipelinePath, setPipelinePath] = useState(project.PipelinePath);
  const [ForkPolicy, setForkPolicy] = useState<TForkPolicy>(project.ForkPolicy);
  const [Problems, setProblems] = useState<TIssues>({});
  const archived = project.State === 'archived';
  const locked = !manager || archived || save.isPending;
  const changes = changesOf(project, { Name: Name.trim(), PipelinePath: PipelinePath.trim(), ForkPolicy });
  const dirty = Object.keys(changes).length > 0;

  const HandleSubmit = () => {
    const parsed = settingsSchema.safeParse({ Name, PipelinePath });
    if (!parsed.success) {
      setProblems(issueText(parsed.error));
      return;
    }
    setProblems({});
    save.mutate(changesOf(project, { ...parsed.data, ForkPolicy }), {
      onSuccess: (updated) => {
        setName(updated.Name);
        setPipelinePath(updated.PipelinePath);
        setForkPolicy(updated.ForkPolicy);
        Push({ Id: `project-saved-${updated.Id}`, Accent: 'live', Title: t('projects.settings.saved') });
      },
    });
  };

  return (
    <Card title={t('projects.settings.title')} icon="settings" padded>
      <form
        noValidate
        className="grid gap-5"
        onSubmit={(event) => {
          event.preventDefault();
          HandleSubmit();
        }}
      >
        {archived ? <InlineAlert accent="queued" title={t('projects.settings.archived')} /> : null}
        {!manager && !archived ? <p className="m-0 text-body-sm text-tertiary">{t('projects.settings.readOnly')}</p> : null}
        {save.error ? <ErrorAlert error={save.error} /> : null}
        <HintedField label={t('projects.form.name')} hint={t('projects.form.name.hint')} error={Problems.Name}>
          <Input maxLength={PROJECT_NAME_MAX} autoComplete="off" value={Name} disabled={locked} onChange={(event) => { setName(event.target.value); }} />
        </HintedField>
        <HintedField label={t('projects.form.path')} hint={t('projects.form.path.hint', { ext: PIPELINE_EXTENSIONS, example: DEFAULT_PIPELINE_PATH })} error={Problems.PipelinePath}>
          <Input
            dir="ltr"
            maxLength={PIPELINE_PATH_MAX}
            autoComplete="off"
            spellCheck={false}
            value={PipelinePath}
            disabled={locked}
            onChange={(event) => { setPipelinePath(event.target.value); }}
          />
        </HintedField>
        <ForkPolicyChoice value={ForkPolicy} allowAutomatic={allowAutomatic || project.ForkPolicy === 'automatic'} disabled={locked} onChange={setForkPolicy} />
        {manager && !archived ? (
          <Button type="submit" variant="primary" className="justify-self-start" disabled={!dirty || save.isPending}>
            {save.isPending ? t('projects.settings.saving') : t('projects.settings.save')}
          </Button>
        ) : null}
      </form>
    </Card>
  );
};

export interface IProjectArchiveProps {
  organizationId: string;
  project: TProject;
}

export const ProjectArchive = ({ organizationId, project }: IProjectArchiveProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const archive = useArchiveProject(organizationId, project.Id);
  const [Confirming, setConfirming] = useState(false);
  return (
    <div className="grid gap-3">
      {Confirming ? <p className="m-0 text-body-sm text-secondary">{t('projects.archive.question', { name: project.Name })}</p> : null}
      {archive.error ? <ErrorAlert error={archive.error} /> : null}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="danger"
          iconStart="queue"
          disabled={archive.isPending}
          onClick={() => {
            if (!Confirming) {
              setConfirming(true);
              return;
            }
            archive.mutate(undefined, {
              onSuccess: () => {
                setConfirming(false);
                Push({ Id: `project-archived-${project.Id}`, Accent: 'live', Title: t('projects.archive.done', { name: project.Name }) });
              },
            });
          }}
        >
          {Confirming ? t('projects.archive.confirm') : t('projects.archive.action')}
        </Button>
        {Confirming ? (
          <Button size="sm" variant="ghost" disabled={archive.isPending} onClick={() => { setConfirming(false); }}>
            {t('projects.cancel')}
          </Button>
        ) : null}
      </div>
    </div>
  );
};

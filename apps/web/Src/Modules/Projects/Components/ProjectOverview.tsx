import type { TRole } from '@/Core/Auth/AuthTypes';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';
import type { TProject } from '@/Modules/Projects/Api/ProjectsApi';
import { RepositoryCard } from '@/Modules/Projects/Components/RepositoryCard';
import { RunsCard } from '@/Modules/Projects/Components/RunsCard';
import { useRepository } from '@/Modules/Projects/Hooks/useRepository';
import { canOnProject, triggerBlock } from '@/Modules/Projects/Types/ProjectAccess';

export interface IProjectOverviewProps {
  organizationId: string;
  project: TProject;
  role: TRole;
  runHref: (runId: string) => string;
  onOpenRun: (runId: string) => void;
  onLink: () => void;
}

export const ProjectOverview = ({ organizationId, project, role, runHref, onOpenRun, onLink }: IProjectOverviewProps) => {
  const t = useT();
  const repository = useRepository(organizationId, project.Id, project.RepositoryLinked);
  const ready =
    project.RepositoryLinked &&
    repository.data !== undefined &&
    repository.data.PendingHostKeyFingerprint === undefined &&
    repository.data.LastDeliveryAt !== undefined;
  return (
    <>
      {project.State === 'archived' ? (
        <Badge tone="mute" icon="queue" className="justify-self-start">
          {t('projects.state.archived')}
        </Badge>
      ) : null}
      {ready ? null : (
        <RepositoryCard
          organizationId={organizationId}
          projectId={project.Id}
          linked={project.RepositoryLinked}
          manager={canOnProject(role, 'project.manage')}
          blocked={project.State === 'archived' ? t('projects.archived.blocked') : null}
          onLink={onLink}
        />
      )}
      <RunsCard organizationId={organizationId} projectId={project.Id} hrefFor={runHref} onOpenRun={onOpenRun} />
    </>
  );
};

export interface IProjectActionProps {
  project: TProject;
  role: TRole;
  onTrigger: () => void;
}

export const ProjectAction = ({ project, role, onTrigger }: IProjectActionProps) => {
  const t = useT();
  const blocked = triggerBlock(project, role);
  return (
    <>
      <span className="flex-1" />
      <span className="grid justify-items-end gap-1 max-md:justify-items-start">
        <Button variant="primary" iconStart="play" disabled={blocked !== null} aria-describedby={blocked === null ? undefined : 'trigger-blocked'} onClick={onTrigger}>
          {t('runs.trigger.open')}
        </Button>
        {blocked === null ? null : (
          <small id="trigger-blocked" className="max-w-72 text-body-sm text-tertiary">
            {t(blocked)}
          </small>
        )}
      </span>
    </>
  );
};

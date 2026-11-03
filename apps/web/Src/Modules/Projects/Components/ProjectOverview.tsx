import type { TRole } from '@/Core/Auth/AuthTypes';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';
import type { TProject } from '@/Modules/Projects/Api/ProjectsApi';
import { RepositoryCard } from '@/Modules/Projects/Components/RepositoryCard';
import { RunsCard } from '@/Modules/Projects/Components/RunsCard';
import { useRepository } from '@/Modules/Projects/Hooks/useRepository';
import { canOnProject } from '@/Modules/Projects/Types/ProjectAccess';

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
  const repository = useRepository(organizationId, project.Id);
  const ready = repository.data !== undefined && repository.data.PendingHostKeyFingerprint === undefined && repository.data.LastDeliveryAt !== undefined;
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
  organizationId: string;
  project: TProject;
  role: TRole;
  onTrigger: () => void;
}

export const ProjectAction = ({ organizationId, project, role, onTrigger }: IProjectActionProps) => {
  const t = useT();
  const repository = useRepository(organizationId, project.Id);
  if (!canOnProject(role, 'run.trigger') || project.State !== 'active' || repository.data === undefined) return null;
  return (
    <>
      <span className="flex-1" />
      <Button variant="primary" iconStart="play" onClick={onTrigger}>
        {t('runs.trigger.open')}
      </Button>
    </>
  );
};

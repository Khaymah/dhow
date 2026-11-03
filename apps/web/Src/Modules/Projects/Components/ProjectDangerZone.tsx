import type { ReactNode } from 'react';

import { useT } from '@/I18n/useT';
import type { TProject } from '@/Modules/Projects/Api/ProjectsApi';
import { RepositoryUnlink } from '@/Modules/Projects/Components/RepositoryActions';
import { ProjectArchive } from '@/Modules/Projects/Components/ProjectSettingsCard';

export interface IProjectDangerZoneProps {
  organizationId: string;
  project: TProject;
}

const DangerItem = ({ title, about, children }: { title: string; about: string; children: ReactNode }) => (
  <li className="grid min-w-0 gap-3 border-t border-line px-card py-4 first:border-t-0">
    <span className="grid min-w-0 gap-1">
      <span className="text-label">{title}</span>
      <small className="text-body-sm text-secondary">{about}</small>
    </span>
    {children}
  </li>
);

export const ProjectDangerZone = ({ organizationId, project }: IProjectDangerZoneProps) => {
  const t = useT();
  return (
    <div className="min-w-0 rounded-md border border-bad-line bg-surface">
      <ul aria-label={t('projects.danger.title')} className="m-0 grid list-none p-0">
        {!project.RepositoryLinked ? null : (
          <DangerItem title={t('repository.unlink.title')} about={t('repository.unlink.about')}>
            <RepositoryUnlink organizationId={organizationId} projectId={project.Id} />
          </DangerItem>
        )}
        <DangerItem title={t('projects.archive.title')} about={t('projects.archive.about')}>
          <ProjectArchive organizationId={organizationId} project={project} />
        </DangerItem>
      </ul>
    </div>
  );
};

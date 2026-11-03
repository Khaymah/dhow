import { useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { PageGroup } from '@/App/Layout/PageGroup';
import { useSlide } from '@/App/Navigation/useSlide';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { useT } from '@/I18n/useT';
import { ProjectDangerZone } from '@/Modules/Projects/Components/ProjectDangerZone';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';
import { ProjectSettingsCard } from '@/Modules/Projects/Components/ProjectSettingsCard';
import { RepositoryActions } from '@/Modules/Projects/Components/RepositoryActions';
import { RepositoryCard } from '@/Modules/Projects/Components/RepositoryCard';
import { canOnProject, canRunForksWithoutApproval } from '@/Modules/Projects/Types/ProjectAccess';

export const ProjectSettingsPage = () => {
  const t = useT();
  const { open } = useSlide();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('projects.settings.title')}>{body}</Page>}>
      {({ organizationId, project, role }) => {
        const manager = canOnProject(role, 'project.manage');
        const active = project.State === 'active';
        return (
          <Page title={project.Name} lead={t('projects.settings.lead')} nav={<ProjectNav projectId={projectId} current="settings" />}>
            <PageGroup title={t('projects.group.general')} icon="settings">
              <ProjectSettingsCard organizationId={organizationId} project={project} manager={manager} allowAutomatic={canRunForksWithoutApproval(role)} />
            </PageGroup>
            <PageGroup title={t('projects.group.repository')} icon="layers">
              <RepositoryCard
                organizationId={organizationId}
                projectId={projectId}
                manager={manager}
                blocked={active ? null : t('projects.archived.blocked')}
                onLink={() => { open('repository-link'); }}
              />
              {manager && active ? <RepositoryActions organizationId={organizationId} projectId={projectId} /> : null}
            </PageGroup>
            {manager && active ? (
              <PageGroup title={t('projects.danger.title')} lead={t('projects.danger.lead')} icon="alert" danger>
                <ProjectDangerZone organizationId={organizationId} project={project} />
              </PageGroup>
            ) : null}
          </Page>
        );
      }}
    </ProjectGate>
  );
};

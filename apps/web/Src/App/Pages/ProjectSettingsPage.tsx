import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { PageGroup } from '@/App/Layout/PageGroup';
import { useSlide } from '@/App/Navigation/useSlide';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { RegistryGroup } from '@/App/Pages/SecretsGroups';
import { canManageProjectSecrets, canReadSecrets } from '@/Core/Auth/AuthTypes';
import { Card } from '@/Core/Ui/Card';
import { LinkRow } from '@/Core/Ui/LinkRow';
import { Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import { canReadIngested } from '@/Modules/Ingest/Types/IngestAccess';
import { ProjectDangerZone } from '@/Modules/Projects/Components/ProjectDangerZone';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';
import { ProjectSettingsCard } from '@/Modules/Projects/Components/ProjectSettingsCard';
import { RepositoryActions } from '@/Modules/Projects/Components/RepositoryActions';
import { RepositoryCard } from '@/Modules/Projects/Components/RepositoryCard';
import { canOnProject, canRunForksWithoutApproval } from '@/Modules/Projects/Types/ProjectAccess';

export const ProjectSettingsPage = () => {
  const t = useT();
  const { open } = useSlide();
  const navigate = useNavigate();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('projects.settings.title')}>{body}</Page>}>
      {({ organizationId, project, role }) => {
        const manager = canOnProject(role, 'project.manage');
        const active = project.State === 'active';
        return (
          <Page title={project.Name} lead={t('projects.settings.lead')} nav={<ProjectNav projectId={projectId} current="settings" role={role} />}>
            <PageGroup title={t('projects.group.general')} icon="settings">
              <ProjectSettingsCard organizationId={organizationId} project={project} manager={manager} allowAutomatic={canRunForksWithoutApproval(role)} />
            </PageGroup>
            <PageGroup title={t('projects.group.repository')} icon="layers">
              <RepositoryCard
                organizationId={organizationId}
                projectId={projectId}
                linked={project.RepositoryLinked}
                manager={manager}
                blocked={active ? null : t('projects.archived.blocked')}
                onLink={() => { open('repository-link'); }}
              />
              {manager && active && project.RepositoryLinked ? <RepositoryActions organizationId={organizationId} projectId={projectId} /> : null}
            </PageGroup>
            {canReadIngested(role) ? (
              <PageGroup title={t('ingest.settings.title')} lead={t('ingest.settings.lead')} icon="download">
                <Card>
                  <Rows label={t('ingest.settings.title')}>
                    <LinkRow
                      icon="key"
                      title={t('ingest.settings.link')}
                      sub={t('ingest.settings.sub')}
                      href={`/projects/${projectId}/ingest`}
                      onOpen={() => {
                        void navigate({ to: '/projects/$projectId/ingest', params: { projectId } });
                      }}
                    />
                  </Rows>
                </Card>
              </PageGroup>
            ) : null}
            {canReadSecrets(role) ? (
              <RegistryGroup scope={{ OrganizationId: organizationId, ProjectId: projectId }} writable={canManageProjectSecrets(role) && active} />
            ) : null}
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

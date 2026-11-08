import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { AnchorTarget } from '@/App/Layout/AnchorTarget';
import { SettingsGroup } from '@/App/Layout/SettingsGroup';
import { SettingsToc } from '@/App/Layout/SettingsToc';
import { useSlide } from '@/App/Navigation/useSlide';
import { settingEntry } from '@/App/Navigation/SettingsRegistry';
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
            <SettingsToc page="project" context={{ role, owner: false, projectActive: active }} />
            <SettingsGroup setting="project.general">
              <ProjectSettingsCard organizationId={organizationId} project={project} manager={manager} allowAutomatic={canRunForksWithoutApproval(role)} />
            </SettingsGroup>
            <SettingsGroup setting="project.repository">
              <RepositoryCard
                organizationId={organizationId}
                projectId={projectId}
                linked={project.RepositoryLinked}
                manager={manager}
                blocked={active ? null : t('projects.archived.blocked')}
                onLink={() => { open('repository-link'); }}
              />
              {manager && active && project.RepositoryLinked ? <RepositoryActions organizationId={organizationId} projectId={projectId} /> : null}
            </SettingsGroup>
            {canReadIngested(role) ? (
              <SettingsGroup setting="project.ingest" lead={t('ingest.settings.lead')}>
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
              </SettingsGroup>
            ) : null}
            {canReadSecrets(role) ? (
              <AnchorTarget anchor={settingEntry('project.registry').anchor ?? ''}>
                <RegistryGroup scope={{ OrganizationId: organizationId, ProjectId: projectId }} writable={canManageProjectSecrets(role) && active} />
              </AnchorTarget>
            ) : null}
            {manager && active ? (
              <SettingsGroup setting="project.danger" lead={t('projects.danger.lead')} danger>
                <ProjectDangerZone organizationId={organizationId} project={project} />
              </SettingsGroup>
            ) : null}
          </Page>
        );
      }}
    </ProjectGate>
  );
};

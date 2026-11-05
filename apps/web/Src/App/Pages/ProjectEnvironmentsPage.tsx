import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { useSlide } from '@/App/Navigation/useSlide';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { EnvironmentsCard } from '@/Modules/Environments/Components/EnvironmentsCard';
import { canManageEnvironments } from '@/Modules/Environments/Types/EnvironmentAccess';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';

export const ProjectEnvironmentsPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { open } = useSlide();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('projects.nav.environments')}>{body}</Page>}>
      {({ organizationId, project, role }) => {
        const active = project.State === 'active';
        return (
          <Page
            title={project.Name}
            lead={t('environments.lead')}
            nav={<ProjectNav projectId={projectId} current="environments" role={role} />}
          >
            {active ? null : <InlineAlert accent="building" title={t('projects.archived.blocked')} />}
            <EnvironmentsCard
              organizationId={organizationId}
              projectId={projectId}
              canCreate={canManageEnvironments(role) && active}
              hrefFor={(name) => `/projects/${projectId}/environments/${name}`}
              onOpen={(environmentName) => {
                void navigate({ to: '/projects/$projectId/environments/$environmentName', params: { projectId, environmentName } });
              }}
              onNew={() => {
                open('environment-new');
              }}
            />
          </Page>
        );
      }}
    </ProjectGate>
  );
};

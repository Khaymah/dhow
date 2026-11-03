import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { useSlide } from '@/App/Navigation/useSlide';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { useT } from '@/I18n/useT';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';
import { ProjectAction, ProjectOverview } from '@/Modules/Projects/Components/ProjectOverview';

export const ProjectPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { open } = useSlide();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('projects.detail.title')}>{body}</Page>}>
      {({ organizationId, project, role }) => (
        <Page
          title={project.Name}
          lead={t('projects.detail.lead', { slug: project.Slug, path: project.PipelinePath })}
          action={<ProjectAction project={project} role={role} onTrigger={() => { open('run-trigger'); }} />}
          nav={<ProjectNav projectId={projectId} current="runs" />}
        >
          <ProjectOverview
            organizationId={organizationId}
            project={project}
            role={role}
            runHref={(runId) => `/projects/${projectId}/runs/${runId}`}
            onOpenRun={(runId) => {
              void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId, runId } });
            }}
            onLink={() => { open('repository-link'); }}
          />
        </Page>
      )}
    </ProjectGate>
  );
};

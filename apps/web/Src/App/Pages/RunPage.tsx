import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { isUuid } from '@/App/Pages/Params';
import { Icon } from '@/Core/Ui/Icon';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';
import { RunBody } from '@/Modules/Projects/Components/RunBody';

export const RunPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { projectId, runId } = useParams({ strict: false });
  if (!isUuid(projectId) || !isUuid(runId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('runs.detail.title')}>{body}</Page>}>
      {({ organizationId, project, role }) => (
        <Page title={t('runs.detail.title')} lead={project.Name}>
          <a
            href={`/projects/${projectId}`}
            onClick={followInApp(() => {
              void navigate({ to: '/projects/$projectId', params: { projectId } });
            })}
            className="inline-flex items-center gap-1.5 justify-self-start rounded-xs text-label-sm text-secondary no-underline hover:text-primary"
          >
            <Icon name="back" size={16} />
            {t('runs.detail.back', { name: project.Name })}
          </a>
          <RunBody
            key={runId}
            organizationId={organizationId}
            projectId={projectId}
            runId={runId}
            role={role}
            crumbKey={`${projectId}/runs/${runId}`}
            jobHref={(jobId) => `/jobs/${jobId}`}
            onOpenJob={(jobId) => {
              void navigate({ to: '/jobs/$jobId', params: { jobId } });
            }}
            onSecondFactor={() => {
              void navigate({ to: '/account/security' });
            }}
          />
        </Page>
      )}
    </ProjectGate>
  );
};

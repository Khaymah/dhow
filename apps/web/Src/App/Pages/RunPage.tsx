import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { useSlide } from '@/App/Navigation/useSlide';
import { isUuid } from '@/App/Pages/Params';
import { decisionSlide } from '@/App/Panels';
import { Icon } from '@/Core/Ui/Icon';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import { ApprovalsCard } from '@/Modules/Approvals/Components/ApprovalsCard';
import { JobGate } from '@/Modules/Approvals/Components/JobGate';
import { useRunGate } from '@/Modules/Approvals/Hooks/useRunGate';
import { gateKindOf, isLeader } from '@/Modules/Approvals/Types/Gate';
import { ProjectGate, type IProjectView } from '@/Modules/Projects/Components/ProjectGate';
import { RunBody } from '@/Modules/Projects/Components/RunBody';
import { useRun } from '@/Modules/Projects/Hooks/useRuns';
import { isFinalRun, shortRef } from '@/Modules/Projects/Types/RunStates';

const RunView = ({ view, runId }: { view: IProjectView; runId: string }) => {
  const t = useT();
  const navigate = useNavigate();
  const { organizationId, project, role } = view;
  const projectId = project.Id;
  const run = useRun(organizationId, projectId, runId);
  const { open } = useSlide();
  const jobs = run.data?.Jobs ?? [];
  const gate = useRunGate({
    organizationId,
    projectId,
    runId,
    jobs,
    role,
    live: run.data !== undefined && !isFinalRun(run.data.Run.State),
  });
  const title = run.data === undefined ? t('runs.detail.title') : t('runs.row.number', { number: run.data.Run.Number });

  return (
    <Page title={title} lead={project.Name}>
      <a
        href={`/projects/${projectId}`}
        onClick={followInApp(() => {
          void navigate({ to: '/projects/$projectId', params: { projectId } });
        })}
        className="inline-flex min-h-11 items-center gap-1.5 justify-self-start rounded-xs text-label-sm text-secondary no-underline hover:text-primary"
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
        linked={project.RepositoryLinked}
        crumbKey={`${projectId}/runs/${runId}`}
        jobHref={(jobId) => `/jobs/${jobId}`}
        onOpenJob={(jobId) => {
          void navigate({ to: '/jobs/$jobId', params: { jobId } });
        }}
        onSecondFactor={() => {
          void navigate({ to: '/account/security' });
        }}
        jobExtra={(job) =>
          gateKindOf(job) === null ? null : (
            <JobGate
              job={job}
              refName={shortRef(run.data?.Run.Ref ?? '')}
              role={role}
              organizationId={organizationId}
              projectId={projectId}
              gate={gate}
              leader={isLeader(jobs, job)}
              environmentHref={(environment) => `/projects/${projectId}/environments/${environment}`}
              onOpenEnvironment={(environmentName) => {
                void navigate({ to: '/projects/$projectId/environments/$environmentName', params: { projectId, environmentName } });
              }}
              onDecide={(decision, approvalId) => {
                open(decisionSlide(decision, approvalId));
              }}
            />
          )
        }
        extra={<ApprovalsCard approvals={gate.Approvals} />}
      />
    </Page>
  );
};

export const RunPage = () => {
  const t = useT();
  const { projectId, runId } = useParams({ strict: false });
  if (!isUuid(projectId) || !isUuid(runId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('runs.detail.title')}>{body}</Page>}>
      {(view) => <RunView view={view} runId={runId} />}
    </ProjectGate>
  );
};

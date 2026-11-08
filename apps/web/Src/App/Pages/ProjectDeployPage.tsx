import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { MyApprovalsCard } from '@/Modules/Approvals/Components/MyApprovalsCard';
import { DeployBoard } from '@/Modules/Boards/Components/DeployBoard';
import { BoardTargetActions } from '@/Modules/Environments/Components/BoardTargetActions';
import { canActOnTargets } from '@/Modules/Environments/Types/EnvironmentAccess';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';

export const ProjectDeployPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('boards.title')}>{body}</Page>}>
      {({ organizationId, project, role }) => {
        const canAct = canActOnTargets(role);
        return (
          <Page
            title={project.Name}
            lead={t('boards.lead')}
            nav={<ProjectNav projectId={projectId} current="deploy" role={role} />}
          >
            {canAct ? null : <InlineAlert accent="building" title={t('boards.action.forbidden')} />}
            <MyApprovalsCard
              organizationId={organizationId}
              projectId={projectId}
              hideWhenEmpty
              hrefFor={(approval) => `/projects/${approval.ProjectId}/runs/${approval.RunId}`}
              onOpen={(approval) => {
                void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId: approval.ProjectId, runId: approval.RunId } });
              }}
            />
            <DeployBoard
              organizationId={organizationId}
              projectId={projectId}
              renderActions={(environment, target) => (
                <BoardTargetActions
                  organizationId={organizationId}
                  projectId={projectId}
                  environmentName={environment.Name}
                  targetName={target.Name}
                  canAct={canAct}
                  paused={environment.Paused}
                />
              )}
            />
          </Page>
        );
      }}
    </ProjectGate>
  );
};

import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { PageGroup } from '@/App/Layout/PageGroup';
import { useSlide } from '@/App/Navigation/useSlide';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { IngestTokensCard } from '@/Modules/Ingest/Components/IngestTokensCard';
import { ReceivedFilesCard } from '@/Modules/Ingest/Components/ReceivedFilesCard';
import { SendBuildPanel } from '@/Modules/Ingest/Components/SendBuildPanel';
import { canManageIngest, canReadIngested } from '@/Modules/Ingest/Types/IngestAccess';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';

export const ProjectIngestPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { open } = useSlide();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('projects.nav.ingest')}>{body}</Page>}>
      {({ organizationId, project, role }) => {
        const manager = canManageIngest(role);
        const active = project.State === 'active';
        return (
          <Page title={project.Name} lead={t('ingest.lead')} nav={<ProjectNav projectId={projectId} current="ingest" role={role} />}>
            {canReadIngested(role) ? (
              <>
                {active ? null : <InlineAlert accent="building" title={t('projects.archived.blocked')} />}
                {manager ? null : <InlineAlert accent="building" title={t('ingest.readOnly')} />}
                {manager ? (
                  <PageGroup title={t('ingest.tokens.group')} icon="key">
                    <IngestTokensCard
                      organizationId={organizationId}
                      projectId={projectId}
                      writable={active}
                      onNew={() => {
                        open('ingest-token-new');
                      }}
                    />
                  </PageGroup>
                ) : null}
                <PageGroup title={t('ingest.files.group')} icon="file">
                  <ReceivedFilesCard
                    organizationId={organizationId}
                    projectId={projectId}
                    runHref={(runId) => `/projects/${projectId}/runs/${runId}`}
                    onOpenRun={(runId) => {
                      void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId, runId } });
                    }}
                  />
                </PageGroup>
                {manager ? (
                  <PageGroup title={t('ingest.send.group')} icon="terminal">
                    <SendBuildPanel />
                  </PageGroup>
                ) : null}
              </>
            ) : (
              <Card>
                <EmptyState icon="shield" text={t('ingest.forbidden')} />
              </Card>
            )}
          </Page>
        );
      }}
    </ProjectGate>
  );
};

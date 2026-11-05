import { useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { SecretsGroups } from '@/App/Pages/SecretsGroups';
import { canManageProjectSecrets, canReadSecrets } from '@/Core/Auth/AuthTypes';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';

export const ProjectSecretsPage = () => {
  const t = useT();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('projects.nav.secrets')}>{body}</Page>}>
      {({ organizationId, project, role }) => {
        const writable = canManageProjectSecrets(role);
        return (
          <Page
            title={project.Name}
            lead={t('secrets.project.lead')}
            nav={<ProjectNav projectId={projectId} current="secrets" role={role} />}
          >
            {canReadSecrets(role) ? (
              <>
                {writable ? null : <InlineAlert accent="building" title={t('secrets.readOnly')} />}
                <SecretsGroups
                  scope={{
                    OrganizationId: organizationId,
                    ProjectId: projectId,
                  }}
                  writable={writable}
                />
              </>
            ) : (
              <Card>
                <EmptyState icon="shield" text={t('secrets.forbidden')} />
              </Card>
            )}
          </Page>
        );
      }}
    </ProjectGate>
  );
};

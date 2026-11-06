import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { isEnvironmentName, isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { canManageEnvironments } from '@/Modules/Environments/Types/EnvironmentAccess';
import { Card } from '@/Core/Ui/Card';
import { Icon } from '@/Core/Ui/Icon';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import { TargetCreateForm } from '@/Modules/Environments/Components/TargetCreateForm';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';

export const NewTargetPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { projectId, environmentName } = useParams({ strict: false });
  if (!isUuid(projectId) || !isEnvironmentName(environmentName)) return <NotFoundPage />;

  const back = () => {
    void navigate({ to: '/projects/$projectId/environments/$environmentName', params: { projectId, environmentName } });
  };

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('targets.new.title')}>{body}</Page>}>
      {(view) => (
        <Page
          title={t('targets.new.title')}
          lead={`${view.project.Name} / ${environmentName}`}
          nav={<ProjectNav projectId={projectId} current="environments" role={view.role} />}
        >
          <a
            href={`/projects/${projectId}/environments/${environmentName}`}
            onClick={followInApp(back)}
            className="inline-flex min-h-11 items-center gap-1.5 justify-self-start rounded-xs text-label-sm text-secondary no-underline hover:text-primary"
          >
            <Icon name="back" size={16} />
            {t('targets.page.back')}
          </a>
          {canManageEnvironments(view.role) && view.project.State === 'active' ? (
            <Card padded>
              <TargetCreateForm
                scope={{ OrganizationId: view.organizationId, ProjectId: projectId, EnvironmentName: environmentName }}
                onCreated={(targetName) => {
                  void navigate({
                    to: '/projects/$projectId/environments/$environmentName/targets/$targetName',
                    params: { projectId, environmentName, targetName },
                  });
                }}
              />
            </Card>
          ) : (
            <InlineAlert accent="building" title={t('targets.new.forbidden')} />
          )}
        </Page>
      )}
    </ProjectGate>
  );
};

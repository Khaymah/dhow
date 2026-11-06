import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { PageGroup } from '@/App/Layout/PageGroup';
import { isEnvironmentName, isTargetName, isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { isApiError } from '@/Core/Api/ApiError';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useCrumbTitle } from '@/Core/Hooks/useCrumbTitle';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { Icon } from '@/Core/Ui/Icon';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { followInApp } from '@/Core/Ui/PlainClick';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import type { ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { RemoveTargetZone } from '@/Modules/Environments/Components/RemoveTargetZone';
import { TargetEditForm } from '@/Modules/Environments/Components/TargetEditForm';
import { TargetHostsPanel } from '@/Modules/Environments/Components/TargetHostsPanel';
import { TargetKeyPanel } from '@/Modules/Environments/Components/TargetKeyPanel';
import { CheckSection, Header, ReleasesSection } from '@/Modules/Environments/Components/TargetPanel';
import { useEnvironment } from '@/Modules/Environments/Hooks/useEnvironments';
import { useTarget, useTargetCheck, useTargetReleases } from '@/Modules/Environments/Hooks/useTargets';
import { canActOnTargets, canManageEnvironments } from '@/Modules/Environments/Types/EnvironmentAccess';
import { differentReleases } from '@/Modules/Environments/Types/TargetState';
import { ProjectGate, type IProjectView } from '@/Modules/Projects/Components/ProjectGate';

interface ISectionsProps {
  view: IProjectView;
  environmentName: string;
  target: TTarget;
  onRemoved: () => void;
}

const TargetSections = ({ view, environmentName, target, onRemoved }: ISectionsProps) => {
  const t = useT();
  const { organizationId, project, role } = view;
  const projectId = project.Id;
  const scope: ITargetScope = { OrganizationId: organizationId, ProjectId: projectId, EnvironmentName: environmentName };
  const environment = useEnvironment(organizationId, projectId, environmentName);
  const releases = useTargetReleases(organizationId, projectId, environmentName, target.Name);
  const check = useTargetCheck(organizationId, projectId, environmentName, target.Name, null);
  const active = project.State === 'active';
  const manage = canManageEnvironments(role) && active;
  const act = canActOnTargets(role) && active;
  const props = {
    organizationId,
    projectId,
    environmentName,
    target,
    canAct: act,
    paused: environment.data?.Paused ?? false,
  };

  return (
    <>
      <Header target={target} current={releases.data?.CurrentReleaseId} different={differentReleases(releases.data, check.data)} />
      {manage ? null : <InlineAlert accent="building" title={t('targets.manage.readOnly')} />}
      <PageGroup title={t('targets.page.hosts')} lead={t('targets.page.hosts.lead')} icon="server">
        <Card>
          <TargetHostsPanel scope={scope} target={target} canManage={manage} />
        </Card>
      </PageGroup>
      <PageGroup title={t('targets.page.key')} lead={t('targets.page.key.lead')} icon="key">
        <Card>
          <TargetKeyPanel scope={scope} target={target} canManage={manage} />
        </Card>
      </PageGroup>
      {manage ? (
        <PageGroup title={t('targets.page.settings')} lead={t('targets.page.settings.lead')} icon="settings">
          <Card>
            <TargetEditForm key={target.Version} scope={scope} target={target} />
          </Card>
        </PageGroup>
      ) : null}
      <PageGroup title={t('targets.page.check')} lead={t('targets.page.check.lead')} icon="check">
        <Card>
          <div className="pb-card pt-4">
            <CheckSection {...props} />
          </div>
        </Card>
      </PageGroup>
      <PageGroup title={t('targets.page.history')} lead={t('targets.page.history.lead')} icon="queue">
        <Card>
          <div className="pb-card pt-4">
            <ReleasesSection {...props} detailed />
          </div>
        </Card>
      </PageGroup>
      {manage ? (
        <PageGroup title={t('environments.group.danger')} lead={t('environments.group.danger.lead')} icon="alert" danger>
          <RemoveTargetZone scope={scope} target={target} onRemoved={onRemoved} />
        </PageGroup>
      ) : null}
    </>
  );
};

const TargetView = ({ view, environmentName, targetName }: { view: IProjectView; environmentName: string; targetName: string }) => {
  const t = useT();
  const errorText = useApiErrorText();
  const navigate = useNavigate();
  const projectId = view.project.Id;
  const query = useTarget(view.organizationId, projectId, environmentName, targetName);
  useCrumbTitle(`${projectId}/environments/${environmentName}/targets/${targetName}`, targetName);

  if (isApiError(query.error) && query.error.Status === 404) {
    return (
      <Card>
        <EmptyState icon="server" text={t('targets.page.notFound')} />
      </Card>
    );
  }
  if (query.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('targets.page.error')}>
          {errorText(query.error)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void query.refetch();
          }}
        >
          {t('status.retry')}
        </Button>
      </div>
    );
  }
  if (query.data === undefined) return <Skeleton label={t('targets.page.loading')} lines={6} />;
  return (
    <TargetSections
      view={view}
      environmentName={environmentName}
      target={query.data}
      onRemoved={() => {
        void navigate({ to: '/projects/$projectId/environments/$environmentName', params: { projectId, environmentName } });
      }}
    />
  );
};

export const TargetPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { projectId, environmentName, targetName } = useParams({ strict: false });
  if (!isUuid(projectId) || !isEnvironmentName(environmentName) || !isTargetName(targetName)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('targets.page.title')}>{body}</Page>}>
      {(view) => (
        <Page
          title={targetName}
          lead={`${view.project.Name} / ${environmentName}`}
          nav={<ProjectNav projectId={projectId} current="environments" role={view.role} />}
        >
          <a
            href={`/projects/${projectId}/environments/${environmentName}`}
            onClick={followInApp(() => {
              void navigate({ to: '/projects/$projectId/environments/$environmentName', params: { projectId, environmentName } });
            })}
            className="inline-flex min-h-11 items-center gap-1.5 justify-self-start rounded-xs text-label-sm text-secondary no-underline hover:text-primary"
          >
            <Icon name="back" size={16} />
            {t('targets.page.back')}
          </a>
          <TargetView key={targetName} view={view} environmentName={environmentName} targetName={targetName} />
        </Page>
      )}
    </ProjectGate>
  );
};

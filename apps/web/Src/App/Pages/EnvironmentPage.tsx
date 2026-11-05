import { useNavigate, useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { PageGroup } from '@/App/Layout/PageGroup';
import { isEnvironmentName, isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { SecretsGroups } from '@/App/Pages/SecretsGroups';
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
import { DeploymentsCard } from '@/Modules/Environments/Components/DeploymentsCard';
import { PauseCard } from '@/Modules/Environments/Components/PauseCard';
import { RemoveZone } from '@/Modules/Environments/Components/RemoveZone';
import { RulesForm } from '@/Modules/Environments/Components/RulesForm';
import { RulesSummary } from '@/Modules/Environments/Components/RulesSummary';
import type { TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { useEnvironment } from '@/Modules/Environments/Hooks/useEnvironments';
import {
  canManageEnvironments,
  canPauseEnvironments,
  canReadEnvironmentSecrets,
} from '@/Modules/Environments/Types/EnvironmentAccess';
import { ProjectGate, type IProjectView } from '@/Modules/Projects/Components/ProjectGate';

interface ISectionsProps {
  view: IProjectView;
  environment: TEnvironment;
  onReload: () => void;
  onRemoved: () => void;
}

const EnvironmentSections = ({ view, environment, onReload, onRemoved }: ISectionsProps) => {
  const t = useT();
  const navigate = useNavigate();
  const { organizationId, project, role } = view;
  const projectId = project.Id;
  const manager = canManageEnvironments(role) && project.State === 'active';

  return (
    <>
      {environment.Paused ? (
        <InlineAlert accent="awaiting" title={t('environments.pause.active')}>
          {environment.PausedReason ?? ''}
        </InlineAlert>
      ) : null}
      <PageGroup title={t('environments.group.rules')} lead={t('environments.group.rules.lead')} icon="shield">
        {manager ? (
          <RulesForm organizationId={organizationId} projectId={projectId} environment={environment} onReload={onReload} />
        ) : (
          <>
            {canManageEnvironments(role) ? null : <InlineAlert accent="building" title={t('environments.rules.readOnly')} />}
            <RulesSummary environment={environment} />
          </>
        )}
      </PageGroup>
      {canPauseEnvironments(role) || environment.Paused ? (
        <PageGroup title={t('environments.group.pause')} lead={t('environments.group.pause.lead')} icon="stop">
          <PauseCard
            organizationId={organizationId}
            projectId={projectId}
            environment={environment}
            canPause={canPauseEnvironments(role) && project.State === 'active'}
            canResume={manager}
          />
        </PageGroup>
      ) : null}
      {canReadEnvironmentSecrets(role) ? (
        <>
          {manager ? null : <InlineAlert accent="building" title={t('secrets.readOnly')} />}
          <SecretsGroups
            key={environment.Name}
            scope={{ OrganizationId: organizationId, ProjectId: projectId, EnvironmentName: environment.Name }}
            writable={manager}
          />
        </>
      ) : null}
      <PageGroup title={t('environments.group.deployments')} lead={t('environments.group.deployments.lead')} icon="queue">
        <DeploymentsCard
          organizationId={organizationId}
          projectId={projectId}
          environmentName={environment.Name}
          hrefFor={(runId) => `/projects/${projectId}/runs/${runId}`}
          onOpenRun={(runId) => {
            void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId, runId } });
          }}
        />
      </PageGroup>
      {manager ? (
        <PageGroup title={t('environments.group.danger')} lead={t('environments.group.danger.lead')} icon="alert" danger>
          <RemoveZone organizationId={organizationId} projectId={projectId} environment={environment} onRemoved={onRemoved} />
        </PageGroup>
      ) : null}
    </>
  );
};

const EnvironmentView = ({ view, environmentName }: { view: IProjectView; environmentName: string }) => {
  const t = useT();
  const errorText = useApiErrorText();
  const navigate = useNavigate();
  const projectId = view.project.Id;
  const query = useEnvironment(view.organizationId, projectId, environmentName);
  useCrumbTitle(`${projectId}/environments/${environmentName}`, environmentName);

  if (isApiError(query.error) && query.error.Status === 404) {
    return (
      <Card>
        <EmptyState icon="layers" text={t('environments.notFound')} />
      </Card>
    );
  }
  if (query.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('environments.detail.error')}>
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
  if (query.data === undefined) return <Skeleton label={t('environments.detail.loading')} lines={6} />;
  return (
    <EnvironmentSections
      view={view}
      environment={query.data}
      onReload={() => {
        void query.refetch();
      }}
      onRemoved={() => {
        void navigate({ to: '/projects/$projectId/environments', params: { projectId } });
      }}
    />
  );
};

export const EnvironmentPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { projectId, environmentName } = useParams({ strict: false });
  if (!isUuid(projectId) || !isEnvironmentName(environmentName)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('environments.detail.title')}>{body}</Page>}>
      {(view) => (
        <Page
          title={environmentName}
          lead={view.project.Name}
          nav={<ProjectNav projectId={projectId} current="environments" role={view.role} />}
        >
          <a
            href={`/projects/${projectId}/environments`}
            onClick={followInApp(() => {
              void navigate({ to: '/projects/$projectId/environments', params: { projectId } });
            })}
            className="inline-flex min-h-11 items-center gap-1.5 justify-self-start rounded-xs text-label-sm text-secondary no-underline hover:text-primary"
          >
            <Icon name="back" size={16} />
            {t('environments.back')}
          </a>
          <EnvironmentView key={environmentName} view={view} environmentName={environmentName} />
        </Page>
      )}
    </ProjectGate>
  );
};

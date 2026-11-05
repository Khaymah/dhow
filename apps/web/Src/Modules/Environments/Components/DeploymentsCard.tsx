import { useState } from 'react';

import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import { Pager } from '@/Core/Ui/Pager';
import { Rows } from '@/Core/Ui/Row';
import { StatusDot } from '@/Core/Ui/StatusDot';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useT } from '@/I18n/useT';
import { DEPLOYMENTS_PAGE_SIZE, type TDeployment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { LinkRow } from '@/Modules/Environments/Components/LinkRow';
import { useDeployments } from '@/Modules/Environments/Hooks/useEnvironments';
import { deploymentStatusOf, shortRef, shortSha } from '@/Modules/Environments/Types/DeploymentState';

export interface IDeploymentsCardProps {
  organizationId: string;
  projectId: string;
  environmentName: string;
  hrefFor: (runId: string) => string;
  onOpenRun: (runId: string) => void;
}

const DeploymentRow = ({ deployment, href, onOpen }: { deployment: TDeployment; href: string; onOpen: () => void }) => {
  const t = useT();
  const time = useRelativeTime();
  const sha = shortSha(deployment.Sha);

  return (
    <LinkRow
      icon="play"
      href={href}
      onOpen={onOpen}
      title={
        <span className="inline-flex min-w-0 flex-wrap items-center gap-x-2">
          <span>{t('environments.deployment.run', { number: deployment.RunNumber })}</span>
          <bdi dir="ltr" className="font-mono text-secondary">
            {shortRef(deployment.Ref)}
          </bdi>
          {sha === '' ? null : (
            <bdi dir="ltr" className="font-mono text-tertiary">
              {sha}
            </bdi>
          )}
        </span>
      }
      sub={
        <>
          <bdi dir="ltr" className="font-mono">
            {deployment.JobKey}
          </bdi>
          {' · '}
          {time.ago(deployment.CreatedAt)}
        </>
      }
      end={<StatusDot status={deploymentStatusOf(deployment.State)}>{t(`environments.deployment.state.${deployment.State}`)}</StatusDot>}
    />
  );
};

export const DeploymentsCard = ({ organizationId, projectId, environmentName, hrefFor, onOpenRun }: IDeploymentsCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Page, setPage] = useState(1);
  const query = useDeployments(organizationId, projectId, environmentName, Page);
  const page = query.data;

  return (
    <div className="grid gap-3">
      <Card title={t('environments.deployments.list')} icon="queue" end={page === undefined ? undefined : t('environments.deployments.total', { count: page.Meta.Total })}>
        <CardStates
          state={cardStateOf(query, page?.Items.length === 0)}
          loadingLabel={t('environments.deployments.loading')}
          errorTitle={t('environments.deployments.error')}
          errorText={query.error ? errorText(query.error) : undefined}
          retryLabel={t('status.retry')}
          onRetry={() => {
            void query.refetch();
          }}
          emptyIcon="queue"
          emptyText={t('environments.deployments.empty')}
          lines={4}
        >
          <Rows label={t('environments.deployments.list')}>
            {page?.Items.map((deployment) => (
              <DeploymentRow
                key={deployment.Id}
                deployment={deployment}
                href={hrefFor(deployment.RunId)}
                onOpen={() => {
                  onOpenRun(deployment.RunId);
                }}
              />
            ))}
          </Rows>
        </CardStates>
      </Card>
      {page === undefined ? null : (
        <Pager page={page.Meta.Page} limit={page.Meta.Limit || DEPLOYMENTS_PAGE_SIZE} total={page.Meta.Total} busy={query.isFetching} onPage={setPage} />
      )}
    </div>
  );
};

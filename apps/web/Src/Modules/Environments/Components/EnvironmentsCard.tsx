import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import { Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { LinkRow } from '@/Core/Ui/LinkRow';
import { useDeployedNow, useEnvironments } from '@/Modules/Environments/Hooks/useEnvironments';
import { shortRef, shortSha } from '@/Modules/Environments/Types/DeploymentState';

export interface IEnvironmentsCardProps {
  organizationId: string;
  projectId: string;
  canCreate: boolean;
  hrefFor: (name: string) => string;
  onOpen: (name: string) => void;
  onNew: () => void;
}

interface IEnvironmentRowProps {
  organizationId: string;
  projectId: string;
  environment: TEnvironment;
  href: string;
  onOpen: () => void;
}

const DeployedNow = ({ organizationId, projectId, name }: { organizationId: string; projectId: string; name: string }) => {
  const t = useT();
  const time = useRelativeTime();
  const deployed = useDeployedNow(organizationId, projectId, name);
  if (deployed.isError) return <>{t('environments.deployed.error')}</>;
  if (deployed.data === undefined) return <>{t('environments.deployed.loading')}</>;
  if (deployed.data === null) return <>{t('environments.deployed.none')}</>;
  const sha = shortSha(deployed.data.Sha);
  return (
    <>
      {t('environments.deployed.now', { number: deployed.data.RunNumber })}
      {' · '}
      <bdi dir="ltr" className="font-mono">
        {shortRef(deployed.data.Ref)}
      </bdi>
      {sha === '' ? null : (
        <>
          {' · '}
          <bdi dir="ltr" className="font-mono">
            {sha}
          </bdi>
        </>
      )}
      {' · '}
      {time.ago(deployed.data.CreatedAt)}
    </>
  );
};

const EnvironmentRow = ({ organizationId, projectId, environment, href, onOpen }: IEnvironmentRowProps) => {
  const t = useT();
  return (
    <LinkRow
      icon="layers"
      href={href}
      onOpen={onOpen}
      title={
        <bdi dir="ltr" className="font-mono">
          {environment.Name}
        </bdi>
      }
      sub={
        <span className="grid gap-0.5">
          {environment.Description === '' ? null : <span>{environment.Description}</span>}
          <span>
            <DeployedNow organizationId={organizationId} projectId={projectId} name={environment.Name} />
          </span>
          {environment.Paused ? (
            <span className="text-warn-fg">{t('environments.row.pausedReason', { reason: environment.PausedReason ?? '' })}</span>
          ) : null}
        </span>
      }
      end={environment.Paused ? <Badge tone="warn" icon="stop">{t('environments.row.paused')}</Badge> : null}
    />
  );
};

export const EnvironmentsCard = ({ organizationId, projectId, canCreate, hrefFor, onOpen, onNew }: IEnvironmentsCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const query = useEnvironments(organizationId, projectId);
  const items = query.data;
  const add = canCreate ? (
    <Button size="sm" variant="primary" iconStart="plus" onClick={onNew}>
      {t('environments.add')}
    </Button>
  ) : undefined;

  return (
    <Card title={t('environments.list')} icon="layers" end={items === undefined ? undefined : t('environments.count', { count: items.length })}>
      <CardStates
        state={cardStateOf(query, items?.length === 0)}
        loadingLabel={t('environments.loading')}
        errorTitle={t('environments.error')}
        errorText={query.error ? errorText(query.error) : undefined}
        retryLabel={t('status.retry')}
        onRetry={() => {
          void query.refetch();
        }}
        emptyIcon="layers"
        emptyText={canCreate ? t('environments.empty') : t('environments.empty.readOnly')}
        emptyAction={add}
        lines={4}
      >
        <Rows label={t('environments.list')}>
          {items?.map((environment) => (
            <EnvironmentRow
              key={environment.Name}
              organizationId={organizationId}
              projectId={projectId}
              environment={environment}
              href={hrefFor(environment.Name)}
              onOpen={() => {
                onOpen(environment.Name);
              }}
            />
          ))}
        </Rows>
        {add ? <div className="flex border-t border-line px-card py-3">{add}</div> : null}
      </CardStates>
    </Card>
  );
};

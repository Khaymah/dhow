import type { ReactNode } from 'react';

import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TRepository } from '@/Modules/Projects/Api/ProjectsApi';
import { CopyField } from '@/Modules/Projects/Components/CopyField';
import { RepositoryState } from '@/Modules/Projects/Components/RepositoryState';
import { isNotLinked, useRepository } from '@/Modules/Projects/Hooks/useRepository';

export interface IRepositoryCardProps {
  organizationId: string;
  projectId: string;
  linked: boolean;
  manager: boolean;
  blocked: string | null;
  onLink: () => void;
}

const Fact = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="grid min-w-0 gap-1 sm:grid-cols-[10rem_minmax(0,1fr)] sm:gap-3">
    <dt className="text-body-sm text-secondary">{label}</dt>
    <dd className="m-0 min-w-0 text-body-sm text-primary">{children}</dd>
  </div>
);

const Facts = ({ repository }: { repository: TRepository }) => {
  const t = useT();
  const time = useRelativeTime();
  return (
    <dl className="m-0 grid gap-3">
      <Fact label={t('repository.facts.address')}>
        <bdi dir="ltr" className="break-all font-mono">
          {repository.CloneUrl}
        </bdi>
      </Fact>
      <Fact label={t('repository.facts.branch')}>
        <bdi dir="ltr" className="font-mono">
          {repository.DefaultBranch}
        </bdi>
      </Fact>
      <Fact label={t('repository.facts.transport')}>{t(`repository.transport.${repository.Transport}`)}</Fact>
      <Fact label={t('repository.facts.state')}>
        <RepositoryState repository={repository} />
      </Fact>
      <Fact label={t('repository.facts.delivery')}>
        {repository.LastDeliveryAt === undefined ? t('repository.facts.delivery.never') : time.ago(repository.LastDeliveryAt)}
      </Fact>
      {repository.HostKeyFingerprint === undefined ? null : (
        <Fact label={t('repository.facts.hostKey')}>
          <bdi dir="ltr" className="break-all font-mono">
            {repository.HostKeyFingerprint}
          </bdi>
        </Fact>
      )}
    </dl>
  );
};

const Linked = ({ repository, manager, blocked, onLink }: { repository: TRepository } & Omit<IRepositoryCardProps, 'organizationId' | 'projectId' | 'linked'>) => {
  const t = useT();
  const time = useRelativeTime();
  return (
    <div className="grid gap-5">
      <Facts repository={repository} />
      {repository.PendingHostKeyFingerprint === undefined ? null : (
        <InlineAlert accent="awaiting" title={t('repository.pending.title')}>
          {t('repository.pending.body')}
        </InlineAlert>
      )}
      <CopyField id="webhook-url" label={t('repository.webhook.url')} value={repository.WebhookUrl} copy={t('repository.webhook.copy')} copied={t('repository.webhook.copied')} />
      {repository.DeployKeyPublic === undefined ? null : (
        <CopyField
          id="deploy-key"
          label={t('repository.deployKey.label')}
          value={repository.DeployKeyPublic}
          copy={t('repository.deployKey.copy')}
          copied={t('repository.deployKey.copied')}
        />
      )}
      {repository.PreviousDeployKeyExpiresAt === undefined ? null : (
        <InlineAlert accent="awaiting" title={t('repository.previousKey.title')}>
          {t('repository.previousKey.body', { when: time.full(repository.PreviousDeployKeyExpiresAt) })}
        </InlineAlert>
      )}
      {manager && blocked === null ? (
        <Button variant="primary" iconStart={repository.PendingHostKeyFingerprint === undefined ? 'settings' : 'shield'} className="justify-self-start" onClick={onLink}>
          {repository.PendingHostKeyFingerprint === undefined ? t('repository.change') : t('repository.pending.confirm')}
        </Button>
      ) : (
        <p className="m-0 text-body-sm text-tertiary">{blocked ?? t('repository.readOnly')}</p>
      )}
    </div>
  );
};

export const RepositoryCard = ({ organizationId, projectId, linked, manager, blocked, onLink }: IRepositoryCardProps) => {
  const t = useT();
  const repository = useRepository(organizationId, projectId, linked);

  if (linked && repository.isPending) {
    return (
      <Card title={t('repository.title')} icon="layers" padded>
        <Skeleton label={t('repository.loading')} lines={4} />
      </Card>
    );
  }
  if (repository.isError && !isNotLinked(repository.error)) {
    return (
      <Card title={t('repository.title')} icon="layers" padded>
        <div className="grid justify-items-start gap-3">
          <InlineAlert accent="failed" title={t('repository.error.load')} />
          <Button size="sm" onClick={() => void repository.refetch()}>
            {t('projects.retry')}
          </Button>
        </div>
      </Card>
    );
  }
  if (repository.data === undefined) {
    return (
      <Card title={t('repository.title')} icon="layers" padded>
        <EmptyState
          icon="layers"
          text={manager ? t('repository.empty.manager') : t('repository.empty.member')}
          action={
            manager && blocked === null ? (
              <Button variant="primary" iconStart="plus" onClick={onLink}>
                {t('repository.link')}
              </Button>
            ) : blocked === null ? null : (
              <span className="text-body-sm text-tertiary">{blocked}</span>
            )
          }
        />
      </Card>
    );
  }
  return (
    <Card title={t('repository.title')} icon="layers" padded>
      <Linked repository={repository.data} manager={manager} blocked={blocked} onLink={onLink} />
    </Card>
  );
};

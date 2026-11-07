import { useEffect, useState, type ReactNode } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { rotateWebhookSecret } from '@/Modules/Projects/Api/ProjectsApi';
import { WebhookSecretOnce } from '@/Modules/Projects/Components/WebhookSecretOnce';
import { useRepository, useRepositoryAction, useUnlinkRepository } from '@/Modules/Projects/Hooks/useRepository';

export interface IRepositoryActionsProps {
  organizationId: string;
  projectId: string;
}

const ActionRow = ({ title, about, children }: { title: string; about: string; children: ReactNode }) => (
  <li className="grid min-w-0 gap-3 border-t border-line px-card py-4 first:border-t-0">
    <span className="grid min-w-0 gap-1">
      <span className="text-label">{title}</span>
      <small className="text-body-sm text-secondary">{about}</small>
    </span>
    {children}
  </li>
);

const SecretRotation = ({ organizationId, projectId }: IRepositoryActionsProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Secret, setSecret] = useState<string | null>(null);
  const [Confirming, setConfirming] = useState(false);
  const [Busy, setBusy] = useState(false);
  const [Problem, setProblem] = useState<string | null>(null);

  useEffect(
    () => () => {
      setSecret(null);
    },
    [],
  );

  const HandleRotate = () => {
    setBusy(true);
    setProblem(null);
    rotateWebhookSecret(organizationId, projectId)
      .then((result) => {
        setSecret(result.WebhookSecret ?? null);
        setConfirming(false);
      })
      .catch((error: unknown) => {
        setProblem(errorText(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  if (Secret !== null) return <WebhookSecretOnce secret={Secret} onHide={() => { setSecret(null); }} />;
  return (
    <div className="grid gap-3">
      {Problem === null ? null : <InlineAlert accent="failed" title={Problem} />}
      {Confirming ? <p className="m-0 text-body-sm text-secondary">{t('repository.actions.secret.question')}</p> : null}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" iconStart="key" disabled={Busy} onClick={Confirming ? HandleRotate : () => { setConfirming(true); }}>
          {Confirming ? t('repository.actions.secret.confirm') : t('repository.actions.secret.action')}
        </Button>
        {Confirming ? (
          <Button size="sm" variant="ghost" disabled={Busy} onClick={() => { setConfirming(false); }}>
            {t('projects.cancel')}
          </Button>
        ) : null}
      </div>
    </div>
  );
};

const DeployKeyRotation = ({ organizationId, projectId, previous }: IRepositoryActionsProps & { previous: boolean }) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const rotate = useRepositoryAction(organizationId, projectId, 'rotate-deploy-key');
  const confirm = useRepositoryAction(organizationId, projectId, 'confirm-deploy-key');
  const error = rotate.error ?? confirm.error;
  return (
    <div className="grid gap-3">
      {error ? <ErrorAlert error={error} /> : null}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          iconStart="key"
          disabled={rotate.isPending}
          onClick={() => { rotate.mutate(undefined, {
              onSuccess: () => { Push({ Id: 'deploy-key-rotated', Accent: 'live', Title: t('repository.actions.key.done') }); },
            }); }
          }
        >
          {t('repository.actions.key.action')}
        </Button>
        {previous ? (
          <Button size="sm" iconStart="check" disabled={confirm.isPending} onClick={() => { confirm.mutate(); }}>
            {t('repository.actions.key.confirm')}
          </Button>
        ) : null}
      </div>
    </div>
  );
};

export const RepositoryUnlink = ({ organizationId, projectId }: IRepositoryActionsProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const unlink = useUnlinkRepository(organizationId, projectId);
  const [Confirming, setConfirming] = useState(false);
  return (
    <div className="grid gap-3">
      {Confirming ? <p className="m-0 text-body-sm text-secondary">{t('repository.unlink.question')}</p> : null}
      {unlink.error ? <ErrorAlert error={unlink.error} /> : null}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="danger"
          iconStart="trash"
          className="justify-self-start"
          disabled={unlink.isPending}
          onClick={() => {
            if (!Confirming) {
              setConfirming(true);
              return;
            }
            unlink.mutate(undefined, {
              onSuccess: () => {
                setConfirming(false);
                Push({ Id: 'repository-unlinked', Accent: 'live', Title: t('repository.unlink.done') });
              },
            });
          }}
        >
          {Confirming ? t('repository.unlink.confirm') : t('repository.unlink.action')}
        </Button>
        {Confirming ? (
          <Button size="sm" variant="ghost" disabled={unlink.isPending} onClick={() => { setConfirming(false); }}>
            {t('projects.cancel')}
          </Button>
        ) : null}
      </div>
    </div>
  );
};

export const RepositoryActions = ({ organizationId, projectId }: IRepositoryActionsProps) => {
  const t = useT();
  const repository = useRepository(organizationId, projectId, true);
  const data = repository.data;
  if (data === undefined) return null;
  return (
    <Card title={t('repository.actions.title')} icon="key">
      <ul aria-label={t('repository.actions.title')} className="m-0 grid list-none p-0">
        <ActionRow title={t('repository.actions.secret.title')} about={t('repository.actions.secret.about')}>
          <SecretRotation organizationId={organizationId} projectId={projectId} />
        </ActionRow>
        {data.Transport === 'ssh' ? (
          <ActionRow title={t('repository.actions.key.title')} about={t('repository.actions.key.about')}>
            <DeployKeyRotation organizationId={organizationId} projectId={projectId} previous={data.PreviousDeployKeyPublic !== undefined} />
          </ActionRow>
        ) : null}
      </ul>
    </Card>
  );
};

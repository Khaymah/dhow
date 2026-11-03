import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { rotateWebhookSecret, type TRepository } from '@/Modules/Projects/Api/ProjectsApi';
import { CopyField } from '@/Modules/Projects/Components/CopyField';
import { RepositoryState } from '@/Modules/Projects/Components/RepositoryState';
import { WebhookSecretOnce } from '@/Modules/Projects/Components/WebhookSecretOnce';
import { useRepository } from '@/Modules/Projects/Hooks/useRepository';
import { EXAMPLES } from '@/Modules/Projects/Types/LinkForm';

export interface IWebhookStepProps {
  organizationId: string;
  projectId: string;
  repository: TRepository;
  secret: string | null;
  onSecret: (secret: string) => void;
  onNext: () => void;
}

export const WebhookStep = ({ organizationId, projectId, repository, secret, onSecret, onNext }: IWebhookStepProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Busy, setBusy] = useState(false);
  const [Problem, setProblem] = useState<string | null>(null);

  const HandleRotate = () => {
    setBusy(true);
    setProblem(null);
    rotateWebhookSecret(organizationId, projectId)
      .then((result) => {
        if (result.WebhookSecret !== undefined) onSecret(result.WebhookSecret);
      })
      .catch((error: unknown) => {
        setProblem(errorText(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <div className="grid gap-5">
      <p className="m-0 text-body-sm text-secondary">{t('repository.webhook.lead')}</p>
      <CopyField id="webhook-url" label={t('repository.webhook.url')} value={repository.WebhookUrl} copy={t('repository.webhook.copy')} copied={t('repository.webhook.copied')} />
      {secret === null ? (
        <div className="grid gap-3">
          <InlineAlert accent="queued" title={t('repository.webhook.kept.title')}>
            {t('repository.webhook.kept.body')}
          </InlineAlert>
          {Problem === null ? null : <InlineAlert accent="failed" title={Problem} />}
          <Button size="sm" iconStart="key" className="justify-self-start" disabled={Busy} onClick={HandleRotate}>
            {t('repository.webhook.rotate')}
          </Button>
        </div>
      ) : (
        <WebhookSecretOnce secret={secret} />
      )}
      <ol className="m-0 grid list-decimal gap-2 ps-5 text-body-sm text-secondary">
        <li>{t('repository.webhook.how.open', { menu: EXAMPLES.webhooksMenu })}</li>
        <li>{t('repository.webhook.how.fill', { type: EXAMPLES.contentType })}</li>
        <li>{t('repository.webhook.how.events')}</li>
      </ol>
      <Button variant="primary" iconStart="next" className="justify-self-start" onClick={onNext}>
        {secret === null ? t('repository.webhook.next') : t('repository.webhook.next.copied')}
      </Button>
    </div>
  );
};

export interface IStatusStepProps {
  organizationId: string;
  projectId: string;
  onDone: () => void;
}

export const StatusStep = ({ organizationId, projectId, onDone }: IStatusStepProps) => {
  const t = useT();
  const time = useRelativeTime();
  const repository = useRepository(organizationId, projectId);
  const data = repository.data;

  return (
    <div className="grid gap-5">
      {data === undefined ? null : <RepositoryState repository={data} />}
      {data?.LastDeliveryAt === undefined ? (
        <InlineAlert accent="awaiting" title={t('repository.status.waiting.title')}>
          {t('repository.status.waiting.body')}
        </InlineAlert>
      ) : (
        <InlineAlert accent="live" title={t('repository.status.delivered.title')}>
          {t('repository.status.delivered.body', { when: time.ago(data.LastDeliveryAt) })}
        </InlineAlert>
      )}
      <p className="m-0 text-body-sm text-secondary">{t('repository.status.test')}</p>
      <Button variant="primary" iconStart="check" className="justify-self-start" onClick={onDone}>
        {t('repository.status.done')}
      </Button>
    </div>
  );
};

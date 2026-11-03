import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { CopyField } from '@/Modules/Projects/Components/CopyField';

export interface IWebhookSecretOnceProps {
  secret: string;
  onHide?: () => void;
}

export const WebhookSecretOnce = ({ secret, onHide }: IWebhookSecretOnceProps) => {
  const t = useT();
  return (
    <div className="grid gap-3">
      <InlineAlert accent="awaiting" title={t('repository.secret.once.title')}>
        {t('repository.secret.once.body')}
      </InlineAlert>
      <CopyField
        id="webhook-secret"
        label={t('repository.secret.label')}
        value={secret}
        copy={t('repository.secret.copy')}
        copied={t('repository.secret.copied')}
      />
      {onHide === undefined ? null : (
        <Button size="sm" variant="ghost" className="justify-self-start" onClick={onHide}>
          {t('repository.secret.hide')}
        </Button>
      )}
    </div>
  );
};

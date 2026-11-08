import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { copyText } from '@/Core/Ui/Clipboard';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';

export interface ISecretCreatedProps {
  secret: string;
  onDone: () => void;
}

export const SecretCreated = ({ secret, onDone }: ISecretCreatedProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);

  return (
    <div className="grid gap-4">
      <InlineAlert accent="awaiting" title={t('notifications.secret.title')}>
        {t('notifications.secret.lead')}
      </InlineAlert>
      <code
        dir="ltr"
        aria-label={t('notifications.secret.value')}
        className="block rounded-md border border-line bg-surface-inset px-3 py-2.5 text-code break-all select-all"
      >
        {secret}
      </code>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="primary"
          iconStart="copy"
          onClick={() => {
            void copyText(secret).then((done) => {
              Push(
                done
                  ? { Id: 'notification-secret-copied', Accent: 'live', Title: t('notifications.secret.copied') }
                  : { Id: 'notification-secret-copy-failed', Accent: 'awaiting', Title: t('notifications.secret.copyFailed') },
              );
            });
          }}
        >
          {t('notifications.secret.copy')}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          {t('notifications.secret.done')}
        </Button>
      </div>
      <p className="m-0 text-body-sm text-secondary">{t('notifications.secret.usage')}</p>
    </div>
  );
};

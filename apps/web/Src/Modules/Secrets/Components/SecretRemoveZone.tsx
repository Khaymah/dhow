import { useState } from 'react';

import { Button } from '@/Core/Ui/Button';
import { Icon } from '@/Core/Ui/Icon';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { removeSecret, type ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';
import { useRemoval } from '@/Modules/Secrets/Hooks/useRemoval';

export interface ISecretRemoveZoneProps {
  scope: ISecretScope;
  name: string;
  disabled: boolean;
  onRemoved: () => void;
}

export const SecretRemoveZone = ({ scope, name, disabled, onRemoved }: ISecretRemoveZoneProps) => {
  const t = useT();
  const removal = useRemoval();
  const [Asking, setAsking] = useState(false);

  return (
    <section aria-labelledby="secret-danger" className="grid gap-3 rounded-md border border-bad-line p-card">
      <h3 id="secret-danger" className="m-0 flex items-center gap-2 text-title-card text-bad-fg">
        <Icon name="alert" size={18} className="text-accent-coral" />
        {t('secrets.danger.title')}
      </h3>
      <p className="m-0 text-body-sm text-secondary">{t('secrets.danger.lead')}</p>
      {removal.Notice ? <InlineAlert accent="failed" title={removal.Notice} /> : null}
      {Asking ? (
        <div className="grid gap-3">
          <p className="m-0 text-body-sm text-secondary">{t('secrets.remove.question', { name })}</p>
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="danger"
              iconStart="trash"
              disabled={disabled || removal.Busy}
              onClick={() => {
                removal.Confirm(name, () => removeSecret(scope, name).then(onRemoved), t('secrets.removed', { name }));
              }}
            >
              {t('secrets.remove.confirm')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={removal.Busy}
              onClick={() => {
                setAsking(false);
              }}
            >
              {t('mfa.cancel')}
            </Button>
          </div>
        </div>
      ) : (
        <Button
          size="sm"
          variant="danger"
          iconStart="trash"
          className="justify-self-start"
          disabled={disabled}
          onClick={() => {
            setAsking(true);
          }}
        >
          {t('secrets.remove')}
        </Button>
      )}
    </section>
  );
};

import { Button } from '@/Core/Ui/Button';
import { Icon } from '@/Core/Ui/Icon';
import { useT } from '@/I18n/useT';
import { removeRegistryCredential, type ISecretScope, type TRegistryCredential } from '@/Modules/Secrets/Api/SecretsApi';
import { RemoveConfirm } from '@/Modules/Secrets/Components/RemoveConfirm';
import { AgoTime, RowFact } from '@/Modules/Secrets/Components/RowFact';
import { SecretsCard } from '@/Modules/Secrets/Components/SecretsCard';
import { useRemoval, type IRemoval } from '@/Modules/Secrets/Hooks/useRemoval';
import { useRegistryCredentials } from '@/Modules/Secrets/Hooks/useSecretsData';

export interface IRegistrySectionProps {
  scope: ISecretScope;
  writable: boolean;
  onNew: () => void;
  onEdit: (credentialId: string) => void;
}

interface ICredentialRowProps {
  credential: TRegistryCredential;
  writable: boolean;
  removal: IRemoval;
  onEdit: (credentialId: string) => void;
}

const CredentialRow = ({ credential, writable, removal, onEdit }: ICredentialRowProps) => {
  const t = useT();

  return (
    <li className="grid min-w-0 gap-3 border-t border-line px-4 py-3 first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Icon name="server" size={18} className="flex-none text-secondary" />
        <bdi dir="ltr" className="min-w-0 flex-1 truncate text-start font-mono text-label">
          {credential.Host}
        </bdi>
        {writable ? (
          <span className="flex flex-wrap items-center gap-1.5">
            <Button
              size="sm"
              variant="ghost"
              iconStart="swap"
              aria-label={t('registry.change.aria', { host: credential.Host })}
              onClick={() => {
                onEdit(credential.Id);
              }}
            >
              {t('registry.change')}
            </Button>
            <Button
              size="sm"
              variant="danger"
              iconStart="trash"
              aria-label={t('registry.remove.aria', { host: credential.Host })}
              onClick={() => {
                removal.Ask(credential.Id);
              }}
            >
              {t('registry.remove')}
            </Button>
          </span>
        ) : null}
      </div>
      <dl className="m-0 grid grid-cols-2 gap-x-4 gap-y-2.5">
        <RowFact label={t('registry.username')}>
          <bdi dir="ltr" className="font-mono">
            {credential.Username}
          </bdi>
        </RowFact>
        <RowFact label={t('registry.updated')}>
          <AgoTime iso={credential.UpdatedAt} />
        </RowFact>
      </dl>
    </li>
  );
};

export const RegistrySection = ({ scope, writable, onNew, onEdit }: IRegistrySectionProps) => {
  const t = useT();
  const query = useRegistryCredentials(scope);
  const removal = useRemoval();

  return (
    <SecretsCard
      icon="server"
      labels={{
        list: 'registry.list',
        loading: 'registry.loading',
        error: 'registry.error',
        empty: 'registry.empty',
        add: 'registry.add',
      }}
      query={query}
      canAdd={writable}
      notice={removal.Notice}
      onAdd={onNew}
    >
      {(items) =>
        items.map((credential) => [
          <CredentialRow key={credential.Id} credential={credential} writable={writable} removal={removal} onEdit={onEdit} />,
          removal.Open === credential.Id ? (
            <RemoveConfirm
              key={`${credential.Id}-remove`}
              question={t('registry.remove.question', {
                host: credential.Host,
              })}
              confirm={t('registry.remove.confirm')}
              busy={removal.Busy}
              onConfirm={() => {
                removal.Confirm(
                  credential.Id,
                  () => removeRegistryCredential(scope, credential.Id),
                  t('registry.removed', { host: credential.Host }),
                );
              }}
              onCancel={removal.Cancel}
            />
          ) : null,
        ])
      }
    </SecretsCard>
  );
};

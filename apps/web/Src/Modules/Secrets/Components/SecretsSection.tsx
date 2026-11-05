import { useSecrets } from '@/Modules/Secrets/Hooks/useSecretsData';
import type { ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';
import { SecretRow } from '@/Modules/Secrets/Components/SecretRow';
import { SecretsCard } from '@/Modules/Secrets/Components/SecretsCard';

export interface ISecretsSectionProps {
  scope: ISecretScope;
  writable: boolean;
  onNew: () => void;
  onEdit: (name: string) => void;
}

export const SecretsSection = ({ scope, writable, onNew, onEdit }: ISecretsSectionProps) => {
  const query = useSecrets(scope);
  return (
    <SecretsCard
      icon="key"
      labels={{
        list: 'secrets.list',
        loading: 'secrets.loading',
        error: 'secrets.error',
        empty: 'secrets.empty',
        add: 'secrets.add',
      }}
      query={query}
      canAdd={writable}
      primary
      onAdd={onNew}
    >
      {(items) => items.map((secret) => <SecretRow key={secret.Name} secret={secret} scope={scope} writable={writable} onEdit={onEdit} />)}
    </SecretsCard>
  );
};

import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Icon } from '@/Core/Ui/Icon';
import { useT } from '@/I18n/useT';
import { removeVariable, type ISecretScope, type TVariable } from '@/Modules/Secrets/Api/SecretsApi';
import { RemoveConfirm } from '@/Modules/Secrets/Components/RemoveConfirm';
import { AgoTime, RowFact } from '@/Modules/Secrets/Components/RowFact';
import { SecretsCard } from '@/Modules/Secrets/Components/SecretsCard';
import { useRemoval, type IRemoval } from '@/Modules/Secrets/Hooks/useRemoval';
import { useVariables } from '@/Modules/Secrets/Hooks/useSecretsData';

export interface IVariablesSectionProps {
  scope: ISecretScope;
  writable: boolean;
  onNew: () => void;
  onEdit: (name: string) => void;
}

interface IVariableRowProps {
  variable: TVariable;
  writable: boolean;
  removal: IRemoval;
  onEdit: (name: string) => void;
}

const VariableRow = ({ variable, writable, removal, onEdit }: IVariableRowProps) => {
  const t = useT();
  const covers = variable.Overrides.includes('organization');

  return (
    <li className="grid min-w-0 gap-3 border-t border-line px-4 py-3 first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Icon name="terminal" size={18} className="flex-none text-secondary" />
        <bdi dir="ltr" className="min-w-0 flex-1 truncate text-start font-mono text-label">
          {variable.Name}
        </bdi>
        <span className="flex flex-wrap items-center gap-1.5">
          {covers ? <Badge tone="info">{t('variables.overrides.organization')}</Badge> : null}
          {writable ? (
            <>
              <Button
                size="sm"
                variant="ghost"
                iconStart="settings"
                aria-label={t('variables.edit.aria', { name: variable.Name })}
                onClick={() => {
                  onEdit(variable.Name);
                }}
              >
                {t('variables.edit')}
              </Button>
              <Button
                size="sm"
                variant="danger"
                iconStart="trash"
                aria-label={t('variables.remove.aria', { name: variable.Name })}
                onClick={() => {
                  removal.Ask(variable.Name);
                }}
              >
                {t('variables.remove')}
              </Button>
            </>
          ) : null}
        </span>
      </div>
      <dl className="m-0 grid gap-x-4 gap-y-2.5 sm:grid-cols-[1fr_auto]">
        <RowFact label={t('variables.value')}>
          <bdi dir="ltr" className="block max-h-24 overflow-auto font-mono text-start break-words whitespace-pre-wrap">
            {variable.Value}
          </bdi>
        </RowFact>
        <RowFact label={t('variables.updated')}>
          <AgoTime iso={variable.UpdatedAt} />
        </RowFact>
      </dl>
    </li>
  );
};

export const VariablesSection = ({ scope, writable, onNew, onEdit }: IVariablesSectionProps) => {
  const t = useT();
  const query = useVariables(scope);
  const removal = useRemoval();

  return (
    <SecretsCard
      icon="terminal"
      labels={{
        list: 'variables.list',
        loading: 'variables.loading',
        error: 'variables.error',
        empty: 'variables.empty',
        add: 'variables.add',
      }}
      query={query}
      canAdd={writable}
      notice={removal.Notice}
      onAdd={onNew}
    >
      {(items) =>
        items.map((variable) => [
          <VariableRow key={variable.Name} variable={variable} writable={writable} removal={removal} onEdit={onEdit} />,
          removal.Open === variable.Name ? (
            <RemoveConfirm
              key={`${variable.Name}-remove`}
              question={t('variables.remove.question', { name: variable.Name })}
              confirm={t('variables.remove.confirm')}
              busy={removal.Busy}
              onConfirm={() => {
                removal.Confirm(variable.Name, () => removeVariable(scope, variable.Name), t('variables.removed', { name: variable.Name }));
              }}
              onCancel={removal.Cancel}
            />
          ) : null,
        ])
      }
    </SecretsCard>
  );
};

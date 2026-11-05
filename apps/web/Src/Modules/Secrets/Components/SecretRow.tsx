import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Icon } from '@/Core/Ui/Icon';
import { useT } from '@/I18n/useT';
import type { ISecretScope, TSecret } from '@/Modules/Secrets/Api/SecretsApi';
import { AgoTime, RowFact } from '@/Modules/Secrets/Components/RowFact';

export interface ISecretRowProps {
  secret: TSecret;
  scope: ISecretScope;
  writable: boolean;
  onEdit: (name: string) => void;
}

export const SecretRow = ({ secret, scope, writable, onEdit }: ISecretRowProps) => {
  const t = useT();
  const organization = scope.ProjectId === null;
  const covers = secret.Overrides.includes('organization');

  return (
    <li className="grid min-w-0 gap-3 border-t border-line px-4 py-3 first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Icon name="key" size={18} className="flex-none text-secondary" />
        <span className="grid min-w-0 flex-1 gap-px">
          <bdi dir="ltr" className="truncate text-start font-mono text-label">
            {secret.Name}
          </bdi>
          {secret.Description ? <small className="text-body-sm text-tertiary">{secret.Description}</small> : null}
        </span>
        <span className="flex flex-wrap items-center gap-1.5">
          {covers ? <Badge tone="info">{t('secrets.overrides.organization')}</Badge> : null}
          {secret.AvailableToPullRequests ? <Badge tone="warn">{t('secrets.pullRequests')}</Badge> : null}
          {writable ? (
            <Button
              size="sm"
              variant="ghost"
              iconStart="swap"
              aria-label={t('secrets.change.aria', { name: secret.Name })}
              onClick={() => {
                onEdit(secret.Name);
              }}
            >
              {t('secrets.change')}
            </Button>
          ) : null}
        </span>
      </div>
      <dl className="m-0 grid grid-cols-2 gap-x-4 gap-y-2.5 sm:grid-cols-4">
        {organization ? (
          <RowFact label={t('secrets.projects')}>
            {secret.Visibility === 'all_projects'
              ? t('secrets.projects.all')
              : t('secrets.projects.selected', {
                  count: secret.ProjectIds.length,
                })}
          </RowFact>
        ) : null}
        <RowFact label={t('secrets.size')}>
          {t('secrets.size.bytes', { count: secret.SizeBytes })} · {t('secrets.version', { version: secret.Version })}
        </RowFact>
        <RowFact label={t('secrets.updated')}>
          <AgoTime iso={secret.UpdatedAt} />
        </RowFact>
        <RowFact label={t('secrets.lastUsed')}>{secret.LastUsedAt ? <AgoTime iso={secret.LastUsedAt} /> : t('secrets.neverUsed')}</RowFact>
      </dl>
    </li>
  );
};

import { Badge } from '@/Core/Ui/Badge';
import { useT } from '@/I18n/useT';
import type { TRepository } from '@/Modules/Projects/Api/ProjectsApi';

export const RepositoryState = ({ repository }: { repository: TRepository }) => {
  const t = useT();
  if (repository.State === 'host_key_unconfirmed') {
    return (
      <Badge tone="warn" icon="alert">
        {t('repository.state.host_key_unconfirmed')}
      </Badge>
    );
  }
  if (repository.LastDeliveryAt === undefined) {
    return (
      <Badge tone="queue" icon="queue">
        {t('repository.state.waiting')}
      </Badge>
    );
  }
  return (
    <Badge tone="ok" icon="check">
      {t('repository.state.ready')}
    </Badge>
  );
};

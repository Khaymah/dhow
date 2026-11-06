import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { TargetPanel } from '@/Modules/Environments/Components/TargetPanel';
import { useTargets } from '@/Modules/Environments/Hooks/useTargets';

export interface ITargetsCardProps {
  organizationId: string;
  projectId: string;
  environmentName: string;
  canAct: boolean;
  paused: boolean;
}

export const TargetsCard = ({ organizationId, projectId, environmentName, canAct, paused }: ITargetsCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const query = useTargets(organizationId, projectId, environmentName);
  const targets = query.data?.Items ?? [];
  const state = cardStateOf(query, targets.length === 0);

  return (
    <div className="grid gap-3">
      {canAct ? null : <InlineAlert accent="building" title={t('targets.forbidden')} />}
      {state === 'ready' ? (
        targets.map((target) => (
          <TargetPanel
            key={target.Id}
            organizationId={organizationId}
            projectId={projectId}
            environmentName={environmentName}
            target={target}
            canAct={canAct}
            paused={paused}
          />
        ))
      ) : (
        <Card>
          <CardStates
            state={state}
            loadingLabel={t('targets.loading')}
            errorTitle={t('targets.error')}
            errorText={query.error ? errorText(query.error) : undefined}
            retryLabel={t('status.retry')}
            onRetry={() => {
              void query.refetch();
            }}
            emptyIcon="server"
            emptyText={t('targets.empty')}
            lines={3}
          />
        </Card>
      )}
    </div>
  );
};

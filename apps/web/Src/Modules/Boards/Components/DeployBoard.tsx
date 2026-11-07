import type { ReactNode } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Badge } from '@/Core/Ui/Badge';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TBoardEnvironment, TBoardTarget } from '@/Modules/Boards/Api/BoardsApi';
import { TargetRow } from '@/Modules/Boards/Components/TargetRow';
import { useDeployBoard } from '@/Modules/Boards/Hooks/useDeployBoard';

export interface IDeployBoardProps {
  organizationId: string;
  projectId: string;
  renderActions?: (environment: TBoardEnvironment, target: TBoardTarget) => ReactNode;
}

const EnvironmentSection = ({ environment, renderActions }: { environment: TBoardEnvironment; renderActions: IDeployBoardProps['renderActions'] }) => {
  const t = useT();
  return (
    <li data-environment={environment.Name} className="min-w-0 border-t border-line first:border-t-0">
      <div className="flex flex-wrap items-center gap-2 px-card pt-3">
        <h4 className="m-0 min-w-0 text-title-card">
          <bdi>{environment.Name}</bdi>
        </h4>
        {environment.Paused ? (
          <Badge tone="warn" icon="stop">
            {t('boards.env.paused')}
          </Badge>
        ) : null}
        {environment.PendingApprovals > 0 ? (
          <Badge tone="warn" icon="shield">
            {t('boards.env.approvals', { count: environment.PendingApprovals })}
          </Badge>
        ) : null}
      </div>
      {environment.Targets.length === 0 ? (
        <p className="m-0 px-card py-3 text-body-sm text-tertiary">{t('boards.noTargets')}</p>
      ) : (
        <ul aria-label={environment.Name} className="m-0 grid list-none p-0">
          {environment.Targets.map((target) => (
            <TargetRow
              key={target.Id}
              environment={environment}
              target={target}
              actions={renderActions?.(environment, target)}
            />
          ))}
        </ul>
      )}
    </li>
  );
};

export const DeployBoard = ({ organizationId, projectId, renderActions }: IDeployBoardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const board = useDeployBoard(organizationId, projectId);
  const environments = board.data?.Environments ?? [];
  const targets = environments.reduce((sum, environment) => sum + environment.Targets.length, 0);

  return (
    <Card
      title={t('boards.list')}
      icon="layers"
      end={board.data === undefined ? null : t('boards.count', { count: targets })}
    >
      {board.data?.Truncated ? (
        <div className="px-card pb-2">
          <InlineAlert accent="awaiting" title={t('boards.truncated')} />
        </div>
      ) : null}
      <CardStates
        state={cardStateOf(board, environments.length === 0)}
        loadingLabel={t('boards.loading')}
        errorTitle={t('boards.error')}
        errorText={board.error ? errorText(board.error) : undefined}
        retryLabel={t('status.retry')}
        onRetry={() => {
          void board.refetch();
        }}
        emptyIcon="layers"
        emptyText={t('boards.empty')}
      >
        <ul aria-label={t('boards.list')} className="m-0 grid list-none p-0">
          {environments.map((environment) => (
            <EnvironmentSection key={environment.Id} environment={environment} renderActions={renderActions} />
          ))}
        </ul>
      </CardStates>
    </Card>
  );
};

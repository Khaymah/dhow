import type { ReactNode } from 'react';

import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TBoardEnvironment, TBoardTarget } from '@/Modules/Boards/Api/BoardsApi';
import { BatchProgress } from '@/Modules/Boards/Components/BatchProgress';
import { AttentionBadges, LastStatus } from '@/Modules/Boards/Components/TargetBadges';
import { deployedLabelOf } from '@/Modules/Boards/Types/BoardStatus';

export interface ITargetRowProps {
  environment: TBoardEnvironment;
  target: TBoardTarget;
  actions?: ReactNode;
}

export const TargetRow = ({ environment, target, actions }: ITargetRowProps) => {
  const t = useT();
  const deployed = deployedLabelOf(target);

  return (
    <li data-target={target.Name} className="grid min-w-0 gap-3 border-t border-line px-card py-3 first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-start gap-x-6 gap-y-3">
        <div className="grid min-w-0 flex-1 basis-44 gap-0.5">
          <span className="min-w-0 text-label">
            <bdi>{target.Name}</bdi>
          </span>
          <small className="text-body-sm text-tertiary">{t('boards.target.hosts', { count: target.Hosts })}</small>
        </div>
        <div className="grid min-w-0 flex-1 basis-44 gap-0.5">
          <small className="text-body-sm text-tertiary">{t('boards.deployed')}</small>
          {deployed === '' ? (
            <span className="text-body-sm text-secondary">{t('boards.deployed.none')}</span>
          ) : (
            <bdi dir="ltr" className="font-mono text-body-sm text-primary" data-deployed>
              {deployed}
            </bdi>
          )}
        </div>
        <div className="grid min-w-0 flex-1 basis-44 gap-1">
          <small className="text-body-sm text-tertiary">{t('boards.last')}</small>
          <LastStatus target={target} />
        </div>
        <AttentionBadges target={target} />
      </div>
      {target.Running ? <BatchProgress progress={target.Running} /> : null}
      {target.RollbackFailed ? (
        <InlineAlert accent="failed" title={t('boards.note.rollbackFailed')} />
      ) : target.Partial ? (
        <InlineAlert accent="awaiting" title={t('boards.note.partial')} />
      ) : null}
      {actions === undefined ? null : (
        <div role="group" aria-label={t('boards.action.aria', { target: target.Name, environment: environment.Name })}>
          {actions}
        </div>
      )}
    </li>
  );
};

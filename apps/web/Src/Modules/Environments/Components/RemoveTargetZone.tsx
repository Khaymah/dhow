import { useState } from 'react';

import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { removeTarget, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { TargetFactorPanel } from '@/Modules/Environments/Components/TargetFactorPanel';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';

export interface IRemoveTargetZoneProps {
  scope: ITargetScope;
  target: TTarget;
  onRemoved: () => void;
}

export const RemoveTargetZone = ({ scope, target, onRemoved }: IRemoveTargetZoneProps) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const Push = useToastStore((state) => state.Push);
  const [Asking, setAsking] = useState(false);

  return (
    <section className="grid min-w-0 gap-3 rounded-md border border-bad-line bg-surface p-card">
      <div className="grid gap-1">
        <span className="text-label">{t('targets.remove.title')}</span>
        <small className="text-body-sm text-secondary">{t('targets.remove.about')}</small>
      </div>
      {Asking ? (
        <div className="-mx-card -mb-card">
          <TargetFactorPanel
            danger
            question={t('targets.remove.question', { name: target.Name })}
            confirm={t('targets.remove.confirm')}
            icon="trash"
            run={(factor) => removeTarget(scope, target.Name, factor)}
            onDone={(removed) => {
              refresh.Changed();
              Push({ Id: `target-removed-${removed.Name}`, Accent: 'live', Title: t('targets.removed', { name: removed.Name }) });
              onRemoved();
            }}
            onCancel={() => {
              setAsking(false);
            }}
          />
        </div>
      ) : (
        <Button
          size="sm"
          variant="danger"
          iconStart="trash"
          className="justify-self-start"
          onClick={() => {
            setAsking(true);
          }}
        >
          {t('targets.remove')}
        </Button>
      )}
    </section>
  );
};

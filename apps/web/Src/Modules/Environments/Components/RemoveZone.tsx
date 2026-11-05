import { useState } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { useT } from '@/I18n/useT';
import { removeEnvironment, type TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { useEnvironmentRefresh } from '@/Modules/Environments/Hooks/useEnvironmentRefresh';

export interface IRemoveZoneProps {
  organizationId: string;
  projectId: string;
  environment: TEnvironment;
  onRemoved: () => void;
}

const RemovePanel = ({ organizationId, projectId, environment, methods, onRemoved }: IRemoveZoneProps & { methods: readonly TFactorMethod[] }) => {
  const t = useT();
  const refresh = useEnvironmentRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const [Asking, setAsking] = useState(false);
  const action = useFactorAction<TEnvironment>(methods, async (proof) =>
    removeEnvironment(organizationId, projectId, environment.Name, await proof()),
  );

  if (!Asking) {
    return (
      <Button
        size="sm"
        variant="danger"
        iconStart="trash"
        className="justify-self-start"
        onClick={() => {
          setAsking(true);
        }}
      >
        {t('environments.remove')}
      </Button>
    );
  }
  return (
    <div className="-mx-card -mb-card">
      <ConfirmPanel
        danger
        question={t('environments.remove.question', { name: environment.Name })}
        confirm={t('environments.remove.confirm')}
        icon="trash"
        methods={methods}
        action={action}
        onCancel={() => {
          setAsking(false);
          action.Reset();
        }}
        onConfirm={() => {
          void action.Run().then((removed) => {
            if (removed === null) return;
            refresh.Removed(removed);
            Push({ Id: `environment-removed-${removed.Name}`, Accent: 'live', Title: t('environments.removed', { name: removed.Name }) });
            onRemoved();
          });
        }}
      />
    </div>
  );
};

export const RemoveZone = (props: IRemoveZoneProps) => {
  const t = useT();
  return (
    <section className="grid min-w-0 gap-3 rounded-md border border-bad-line bg-surface p-card">
      <div className="grid gap-1">
        <span className="text-label">{t('environments.remove.title')}</span>
        <small className="text-body-sm text-secondary">{t('environments.remove.about')}</small>
      </div>
      <FactorGate>{(methods) => <RemovePanel {...props} methods={methods} />}</FactorGate>
    </section>
  );
};

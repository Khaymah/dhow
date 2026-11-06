import { useQueryClient } from '@tanstack/react-query';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { deploymentsKey } from '@/Modules/Environments/Api/EnvironmentsApi';
import { rollbackTarget, targetsKey, type TCheckStarted, type TTarget, type TTargetRelease } from '@/Modules/Environments/Api/TargetsApi';

export interface IRollbackPanelProps {
  organizationId: string;
  projectId: string;
  environmentName: string;
  target: TTarget;
  release: TTargetRelease;
  methods: readonly TFactorMethod[];
  onClose: () => void;
}

export const RollbackPanel = ({ organizationId, projectId, environmentName, target, release, methods, onClose }: IRollbackPanelProps) => {
  const t = useT();
  const client = useQueryClient();
  const Push = useToastStore((state) => state.Push);
  const action = useFactorAction<TCheckStarted>(methods, async (proof) =>
    rollbackTarget(organizationId, projectId, environmentName, target.Name, release.ReleaseId, await proof()),
  );

  return (
    <div className="grid gap-3 border-t border-line pt-3">
      {release.ArtifactKept ? null : <InlineAlert accent="awaiting" title={t('targets.rollback.noArtifact')} />}
      <ConfirmPanel
        question={t('targets.rollback.question', { target: target.Name, release: release.ReleaseId })}
        confirm={t('targets.rollback.confirm')}
        icon="swap"
        danger
        methods={methods}
        action={action}
        onCancel={() => {
          action.Reset();
          onClose();
        }}
        onConfirm={() => {
          void action.Run().then((started) => {
            if (started === null) return;
            Push({
              Id: `target-rollback-${started.RunId}`,
              Accent: 'awaiting',
              Title: t('targets.rollback.started', { release: release.ReleaseId }),
            });
            void client.invalidateQueries({ queryKey: targetsKey(organizationId, projectId, environmentName) });
            void client.invalidateQueries({ queryKey: deploymentsKey(organizationId, projectId, environmentName) });
            onClose();
          });
        }}
      />
    </div>
  );
};

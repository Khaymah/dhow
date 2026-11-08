import { useQueryClient } from '@tanstack/react-query';
import { useId, useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import { useAction } from '@/Core/Hooks/useAction';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { startTargetCheck, targetsKey } from '@/Modules/Environments/Api/TargetsApi';
import { CheckSection, ReleasesSection } from '@/Modules/Environments/Components/TargetPanel';
import { useTarget } from '@/Modules/Environments/Hooks/useTargets';

export interface IBoardTargetActionsProps {
  organizationId: string;
  projectId: string;
  environmentName: string;
  targetName: string;
  canAct: boolean;
  paused: boolean;
  onChecked?: () => void;
}

const Details = ({ organizationId, projectId, environmentName, targetName, canAct, paused }: IBoardTargetActionsProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const target = useTarget(organizationId, projectId, environmentName, targetName);
  if (target.isError) return <InlineAlert accent="failed" title={t('targets.error')}>{errorText(target.error)}</InlineAlert>;
  if (target.data === undefined) return <Skeleton label={t('targets.loading')} lines={2} />;
  const props = { organizationId, projectId, environmentName, target: target.data, canAct, paused };
  return (
    <div className="grid gap-4 border-t border-line pt-3">
      {canAct ? <CheckSection {...props} /> : null}
      <ReleasesSection {...props} detailed />
    </div>
  );
};

export const BoardTargetActions = (props: IBoardTargetActionsProps) => {
  const { organizationId, projectId, environmentName, targetName, canAct, onChecked } = props;
  const t = useT();
  const client = useQueryClient();
  const detailsId = useId();
  const Push = useToastStore((state) => state.Push);
  const [Open, setOpen] = useState(false);
  const check = useAction(() => startTargetCheck(organizationId, projectId, environmentName, targetName));

  const HandleCheck = () => {
    void check.Run().then((result) => {
      if (isApiError(result)) return;
      Push({ Id: `target-check-${result.RunId}`, Accent: 'building', Title: t('targets.check.started', { name: targetName }) });
      void client.invalidateQueries({ queryKey: targetsKey(organizationId, projectId, environmentName) });
      onChecked?.();
    });
  };

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2">
        {canAct ? (
          <Button size="sm" iconStart="check" disabled={check.Pending} onClick={HandleCheck}>
            {t('boards.action.check')}
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="ghost"
          iconStart={canAct ? 'swap' : 'eye'}
          aria-expanded={Open}
          aria-controls={detailsId}
          onClick={() => {
            setOpen((value) => !value);
          }}
        >
          {Open ? t('boards.action.hide') : canAct ? t('boards.action.rollback') : t('boards.action.details')}
        </Button>
      </div>
      {check.Failure ? <ErrorAlert error={check.Failure} /> : null}
      <div id={detailsId}>{Open ? <Details {...props} /> : null}</div>
    </div>
  );
};

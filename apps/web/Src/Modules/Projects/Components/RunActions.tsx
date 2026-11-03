import { useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import type { TRole } from '@/Core/Auth/AuthTypes';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TRun } from '@/Modules/Projects/Api/RunsApi';
import { useRunAction, type TRunAction } from '@/Modules/Projects/Hooks/useRuns';
import { canOnProject } from '@/Modules/Projects/Types/ProjectAccess';
import { isCancelableRun, isRerunnable } from '@/Modules/Projects/Types/RunStates';

export interface IRunActionsProps {
  organizationId: string;
  projectId: string;
  run: TRun;
  role: TRole;
  onSecondFactor: () => void;
}

const SECOND_FACTOR_REQUIRED = 'SECOND_FACTOR_REQUIRED';

const ActionError = ({ error, onSecondFactor }: { error: unknown; onSecondFactor: () => void }) => {
  const t = useT();
  const errorText = useApiErrorText();
  if (isApiError(error) && error.Code === SECOND_FACTOR_REQUIRED) {
    return (
      <InlineAlert accent="awaiting" title={t('runs.actions.secondFactor.title')}>
        <span className="grid justify-items-start gap-2">
          {t('runs.actions.secondFactor.body')}
          <Button size="sm" iconStart="shield" onClick={onSecondFactor}>
            {t('runs.actions.secondFactor.action')}
          </Button>
        </span>
      </InlineAlert>
    );
  }
  return <InlineAlert accent="failed" title={errorText(error)} />;
};

export const RunActions = ({ organizationId, projectId, run, role, onSecondFactor }: IRunActionsProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const cancel = useRunAction(organizationId, projectId, run.Id, 'cancel');
  const rerun = useRunAction(organizationId, projectId, run.Id, 'rerun');
  const approve = useRunAction(organizationId, projectId, run.Id, 'approve');
  const [Confirm, setConfirm] = useState<TRunAction | null>(null);
  const busy = cancel.isPending || rerun.isPending || approve.isPending;
  const error = cancel.error ?? rerun.error ?? approve.error;

  const mutations = { cancel, rerun, approve };
  const done = (action: TRunAction) => () => {
    setConfirm(null);
    Push({ Id: `run-${action}-${run.Id}`, Accent: 'live', Title: t(`runs.actions.${action}.done`, { number: run.Number }) });
  };
  const HandleRun = (action: TRunAction) => {
    if (action !== 'rerun' && Confirm !== action) {
      setConfirm(action);
      return;
    }
    mutations[action].mutate(undefined, { onSuccess: done(action) });
  };

  const awaiting = run.State === 'awaiting_approval';
  const canApprove = canOnProject(role, 'run.approve');
  const canCancel = canOnProject(role, 'run.cancel');
  const canRerun = canOnProject(role, 'run.trigger');
  const shown = (awaiting && canApprove) || (isCancelableRun(run) && canCancel) || (isRerunnable(run.State) && canRerun);
  const blocked = (awaiting && !canApprove) || (isCancelableRun(run) && !canCancel) || (isRerunnable(run.State) && !canRerun);

  if (!shown && !blocked) return null;
  return (
    <div className="grid gap-3">
      {Confirm === null ? null : <p className="m-0 text-body-sm text-secondary">{t(`runs.actions.${Confirm}.question`)}</p>}
      {error ? <ActionError error={error} onSecondFactor={onSecondFactor} /> : null}
      <div className="flex flex-wrap gap-2">
        {awaiting && canApprove ? (
          <Button variant="primary" iconStart="shield" disabled={busy} onClick={() => { HandleRun('approve'); }}>
            {Confirm === 'approve' ? t('runs.actions.approve.confirm') : t('runs.actions.approve.action')}
          </Button>
        ) : null}
        {isRerunnable(run.State) && canRerun ? (
          <Button variant="primary" iconStart="play" disabled={busy} onClick={() => { HandleRun('rerun'); }}>
            {t('runs.actions.rerun.action')}
          </Button>
        ) : null}
        {isCancelableRun(run) && canCancel ? (
          <Button variant="danger" iconStart="stop" disabled={busy} onClick={() => { HandleRun('cancel'); }}>
            {Confirm === 'cancel' ? t('runs.actions.cancel.confirm') : t('runs.actions.cancel.action')}
          </Button>
        ) : null}
        {Confirm === null ? null : (
          <Button variant="ghost" disabled={busy} onClick={() => { setConfirm(null); }}>
            {t('projects.cancel')}
          </Button>
        )}
      </div>
      {blocked ? <p className="m-0 text-body-sm text-tertiary">{t(awaiting ? 'runs.actions.approve.forbidden' : 'runs.actions.forbidden')}</p> : null}
      {run.CancelRequestedAt !== undefined && !['succeeded', 'failed', 'canceled', 'skipped'].includes(run.State) ? (
        <p className="m-0 text-body-sm text-secondary">{t('runs.actions.cancel.requested')}</p>
      ) : null}
    </div>
  );
};

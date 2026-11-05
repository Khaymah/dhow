import type { ReactNode } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { DecisionForm } from '@/Modules/Approvals/Components/DecisionForm';
import { useRunApprovals } from '@/Modules/Approvals/Hooks/useApprovals';
import type { TDecision } from '@/Modules/Approvals/Types/Gate';

export interface IDecisionBodyProps {
  projectId?: string;
  runId?: string;
  approvalId: string;
  decision: TDecision;
  onDecided: () => void;
  onDone: () => void;
  onSecondFactor: () => void;
}

interface IFoundProps extends IDecisionBodyProps {
  organizationId: string;
  projectId: string;
  runId: string;
}

const withoutPassword = (methods: readonly TFactorMethod[]): TFactorMethod[] => methods.filter((method) => method !== 'password');

const Found = ({ organizationId, projectId, runId, approvalId, decision, onDecided, onDone, onSecondFactor }: IFoundProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const approvals = useRunApprovals(organizationId, projectId, runId, true, false);
  const approval = approvals.data?.find((item) => item.Id === approvalId);
  let body: ReactNode;

  if (approvals.isError) {
    body = (
      <div className="grid justify-items-start gap-2">
        <InlineAlert accent="failed" title={t('approvals.gate.error')}>
          {errorText(approvals.error)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void approvals.refetch();
          }}
        >
          {t('status.retry')}
        </Button>
      </div>
    );
  } else if (approvals.data === undefined) {
    body = <Skeleton label={t('approvals.gate.loading')} lines={4} />;
  } else if (approval === undefined) {
    body = <InlineAlert accent="awaiting" title={t('approvals.decide.missing')} />;
  } else if (approval.State !== 'pending') {
    body = <InlineAlert accent="awaiting" title={t('approvals.decide.notPending', { state: t(`approvals.state.${approval.State}`) })} />;
  } else {
    body = (
      <FactorGate>
        {(all) => {
          const methods = withoutPassword(all);
          if (methods.length === 0) {
            return (
              <div className="grid justify-items-start gap-3">
                <InlineAlert accent="awaiting" title={t('approvals.factor.none')} />
                <Button size="sm" variant="primary" onClick={onSecondFactor}>
                  {t('approvals.factor.setup')}
                </Button>
              </div>
            );
          }
          return (
            <DecisionForm
              organizationId={organizationId}
              projectId={projectId}
              runId={runId}
              approval={approval}
              decision={decision}
              methods={methods}
              onDecided={onDecided}
              onDone={onDone}
            />
          );
        }}
      </FactorGate>
    );
  }
  return <>{body}</>;
};

export const DecisionBody = (props: IDecisionBodyProps) => {
  const t = useT();
  const membership = useCurrentMembership();
  if (membership === undefined) return <Skeleton label={t('ui.loading')} lines={5} />;
  if (props.projectId === undefined || props.runId === undefined) {
    return <InlineAlert accent="awaiting" title={t('approvals.decide.noRun')} />;
  }
  return <Found {...props} organizationId={membership.OrganizationId} projectId={props.projectId} runId={props.runId} />;
};

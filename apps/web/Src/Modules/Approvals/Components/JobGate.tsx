import type { ReactNode } from 'react';

import type { TRole } from '@/Core/Auth/AuthTypes';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge, type TBadgeTone } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Icon } from '@/Core/Ui/Icon';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { followInApp } from '@/Core/Ui/PlainClick';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TApproval } from '@/Modules/Approvals/Api/ApprovalsApi';
import { useBypassAllowed } from '@/Modules/Approvals/Hooks/useApprovals';
import type { IRunGate } from '@/Modules/Approvals/Hooks/useRunGate';
import { canBypassDeployments, canOpenEnvironmentRules } from '@/Modules/Approvals/Types/ApprovalAccess';
import { gateKindOf, pendingFor, type TDecision, type TGateJob, type TGateKind } from '@/Modules/Approvals/Types/Gate';



export interface IJobGateProps {
  job: TGateJob;
  refName: string;
  role: TRole;
  organizationId: string;
  projectId: string;
  gate: IRunGate;
  leader: boolean;
  environmentHref: (environment: string) => string;
  onOpenEnvironment: (environment: string) => void;
  onDecide: (decision: TDecision, approvalId: string) => void;
}

const TONES: Record<TGateKind, TBadgeTone> = {
  skipped: 'mute',
  approval: 'warn',
  timer: 'info',
  environment_busy: 'queue',
  target_busy: 'queue',
  paused: 'warn',
};

const Frame = ({ kind, label, children }: { kind: TGateKind; label: string; children: ReactNode }) => (
  <div data-gate={kind} className="grid min-w-0 gap-2 rounded-md border border-line bg-surface-inset p-3">
    <div className="flex flex-wrap items-center gap-2">
      <Badge tone={TONES[kind]}>{label}</Badge>
    </div>
    {children}
  </div>
);

const Line = ({ children }: { children: ReactNode }) => <p className="m-0 min-w-0 text-body-sm text-secondary">{children}</p>;

const EnvironmentLink = ({ environment, props }: { environment: string; props: IJobGateProps }) => {
  const t = useT();
  if (!canOpenEnvironmentRules(props.role)) return null;
  return (
    <a
      href={props.environmentHref(environment)}
      onClick={followInApp(() => {
        props.onOpenEnvironment(environment);
      })}
      className="inline-flex min-h-11 items-center gap-1.5 justify-self-start rounded-xs text-label-sm text-primary underline decoration-line-2 underline-offset-4 hover:decoration-accent"
    >
      <Icon name="shield" size={16} />
      {t('approvals.gate.openEnvironment')}
    </a>
  );
};

const BypassButton = ({ props, approval }: { props: IJobGateProps; approval: TApproval }) => {
  const t = useT();
  const allowed = useBypassAllowed(props.organizationId, props.projectId, approval.Environment, canBypassDeployments(props.role));
  if (allowed.data !== true) return null;
  return (
    <Button
      size="sm"
      variant="danger"
      iconStart="alert"
      onClick={() => {
        props.onDecide('bypass', approval.Id);
      }}
    >
      {t('approvals.gate.bypass')}
    </Button>
  );
};

const Actions = ({ props, approval }: { props: IJobGateProps; approval: TApproval }) => {
  const t = useT();
  const decides = props.gate.CanDecide(approval);
  return (
    <div className="flex flex-wrap items-center gap-2">
      {decides ? (
        <>
          <Button
            size="sm"
            variant="primary"
            iconStart="check"
            onClick={() => {
              props.onDecide('approve', approval.Id);
            }}
          >
            {t('approvals.gate.approve')}
          </Button>
          <Button
            size="sm"
            variant="danger"
            iconStart="close"
            onClick={() => {
              props.onDecide('reject', approval.Id);
            }}
          >
            {t('approvals.gate.reject')}
          </Button>
        </>
      ) : null}
      <BypassButton props={props} approval={approval} />
    </div>
  );
};

const ApprovalGate = ({ props }: { props: IJobGateProps }) => {
  const t = useT();
  const time = useRelativeTime();
  const { gate, job } = props;
  const approval = pendingFor(gate.Approvals, job);
  let body: ReactNode;
  if (gate.Failed) {
    body = (
      <div className="grid justify-items-start gap-2">
        <InlineAlert accent="failed" title={t('approvals.gate.error')} />
        <Button size="sm" onClick={gate.Retry}>
          {t('status.retry')}
        </Button>
      </div>
    );
  } else if (gate.Loading) {
    body = <Skeleton label={t('approvals.gate.loading')} lines={2} />;
  } else if (approval === undefined) {
    body = <Line>{t('approvals.gate.approval.missing')}</Line>;
  } else {
    body = (
      <>
        <Line>{t('approvals.gate.approval.since', { time: time.ago(approval.CreatedAt) })}</Line>
        <Line>{t('approvals.gate.approval.expires', { time: time.full(approval.ExpiresAt) })}</Line>
        {gate.CanDecide(approval) ? null : <Line>{t('approvals.gate.approval.reviewers', { environment: approval.Environment })}</Line>}
        {props.leader ? <Actions props={props} approval={approval} /> : <Line>{t('approvals.gate.approval.shared')}</Line>}
      </>
    );
  }
  return (
    <Frame kind="approval" label={t('approvals.gate.approval.badge')}>
      {body}
    </Frame>
  );
};

const GateBody = ({ kind, props }: { kind: TGateKind; props: IJobGateProps }) => {
  const t = useT();
  const time = useRelativeTime();
  const { job } = props;
  const environment = job.Environment ?? '';
  if (kind === 'approval') return <ApprovalGate props={props} />;
  if (kind === 'skipped') {
    return (
      <Frame kind={kind} label={t('approvals.gate.skipped.badge')}>
        <Line>{t('approvals.gate.skipped.body', { ref: props.refName, environment })}</Line>
        <Line>{t('approvals.gate.skipped.hint')}</Line>
        <EnvironmentLink environment={environment} props={props} />
      </Frame>
    );
  }
  if (kind === 'timer') {
    return (
      <Frame kind={kind} label={t('approvals.gate.timer.badge')}>
        <Line>{job.GateOpensAt === undefined ? t('approvals.gate.timer.unknown') : t('approvals.gate.timer.body', { time: time.full(job.GateOpensAt) })}</Line>
      </Frame>
    );
  }
  if (kind === 'environment_busy') {
    return (
      <Frame kind={kind} label={t('approvals.gate.busy.badge')}>
        <Line>{t('approvals.gate.busy.body', { environment })}</Line>
      </Frame>
    );
  }
  if (kind === 'target_busy') {
    return (
      <Frame kind={kind} label={t('approvals.gate.targetBusy.badge')}>
        <Line>{t('approvals.gate.targetBusy.body')}</Line>
      </Frame>
    );
  }
  return (
    <Frame kind={kind} label={t('approvals.gate.paused.badge')}>
      <Line>{t('approvals.gate.paused.body', { environment })}</Line>
      <EnvironmentLink environment={environment} props={props} />
    </Frame>
  );
};

export const JobGate = (props: IJobGateProps) => {
  const kind = gateKindOf(props.job);
  return kind === null ? null : <GateBody kind={kind} props={props} />;
};

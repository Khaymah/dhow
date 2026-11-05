import { useEffect, useRef, useState } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { formatClock } from '@/Core/Format/TimeFormat';
import { byteLength } from '@/Core/Forms/ByteLength';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Field } from '@/Core/Ui/Field';
import { FactorFields } from '@/Core/Ui/FactorFields';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import { approveDeployment, bypassDeployment, rejectDeployment, type TApproval } from '@/Modules/Approvals/Api/ApprovalsApi';
import type { TDecision } from '@/Modules/Approvals/Types/Gate';
import { useApprovalsRefresh } from '@/Modules/Approvals/Hooks/useApprovals';
import { COMMENT_MAX_BYTES, REASON_MAX_BYTES } from '@/Modules/Approvals/Types/ApprovalAccess';

export interface IDecisionFormProps {
  organizationId: string;
  projectId: string;
  runId: string;
  approval: TApproval;
  decision: TDecision;
  methods: readonly TFactorMethod[];
  onDecided: () => void;
  onDone: () => void;
}

const DecisionSummary = ({ approval }: { approval: TApproval }) => {
  const t = useT();
  const time = useRelativeTime();
  return (
    <dl className="m-0 grid grid-cols-2 gap-3 rounded-md border border-line bg-surface-inset p-3">
      <div className="grid gap-0.5">
        <dt className="text-body-sm text-tertiary">{t('approvals.decide.environment')}</dt>
        <dd className="m-0 text-body-sm text-primary">
          <bdi dir="ltr" className="font-mono">
            {approval.Environment}
          </bdi>
        </dd>
      </div>
      <div className="grid gap-0.5">
        <dt className="text-body-sm text-tertiary">{t('approvals.decide.run')}</dt>
        <dd className="m-0 text-body-sm text-primary">{t('runs.row.number', { number: approval.RunNumber })}</dd>
      </div>
      <div className="grid gap-0.5">
        <dt className="text-body-sm text-tertiary">{t('approvals.decide.ref')}</dt>
        <dd className="m-0 text-body-sm text-primary">
          <bdi dir="ltr" className="font-mono">
            {approval.Ref}
          </bdi>
        </dd>
      </div>
      <div className="grid gap-0.5">
        <dt className="text-body-sm text-tertiary">{t('approvals.decide.round')}</dt>
        <dd className="m-0 text-body-sm text-primary">{approval.Round}</dd>
      </div>
      <div className="col-span-2 grid gap-0.5">
        <dt className="text-body-sm text-tertiary">{t('approvals.decide.expires')}</dt>
        <dd className="m-0 text-body-sm text-primary">{time.full(approval.ExpiresAt)}</dd>
      </div>
    </dl>
  );
};

const ICONS = { approve: 'check', reject: 'close', bypass: 'alert' } as const;

const DecisionButton = ({ decision, label, disabled }: { decision: TDecision; label: string; disabled: boolean }) =>
  decision === 'approve' ? (
    <Button type="submit" variant="primary" iconStart={ICONS[decision]} className="justify-self-start" disabled={disabled}>
      {label}
    </Button>
  ) : (
    <Button type="submit" variant="danger" iconStart={ICONS[decision]} className="justify-self-start" disabled={disabled}>
      {label}
    </Button>
  );

export const DecisionForm = ({ organizationId, projectId, runId, approval, decision, methods, onDecided, onDone }: IDecisionFormProps) => {
  const t = useT();
  const refresh = useApprovalsRefresh(organizationId);
  const Push = useToastStore((state) => state.Push);
  const [Text, setText] = useState('');
  const [Problem, setProblem] = useState<string | null>(null);
  const sending = useRef(false);
  const mounted = useRef(true);
  const bypass = decision === 'bypass';

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const max = bypass ? REASON_MAX_BYTES : COMMENT_MAX_BYTES;

  const action = useFactorAction<TApproval>(methods, async (proof) => {
    const factor = await proof();
    const text = Text.trim();
    if (decision === 'approve') return approveDeployment(organizationId, projectId, runId, approval.Id, factor, text);
    if (decision === 'reject') return rejectDeployment(organizationId, projectId, runId, approval.Id, factor, text);
    return bypassDeployment(organizationId, projectId, runId, approval.Id, factor, text);
  });

  const problemOf = (): string | null => {
    const text = Text.trim();
    if (bypass && text === '') return t('approvals.error.reasonEmpty');
    if (byteLength(text) > max) return t(bypass ? 'approvals.error.reasonLong' : 'approvals.error.commentLong');
    return null;
  };

  const HandleSubmit = () => {
    if (sending.current || action.Busy || action.Countdown.Remaining > 0) return;
    const problem = problemOf();
    setProblem(problem);
    if (problem !== null) return;
    sending.current = true;
    void action.Run().then((decided) => {
      sending.current = false;
      if (decided === null) return;
      refresh();
      onDecided();
      if (!mounted.current) return;
      setText('');
      action.Reset();
      Push({
        Id: `approval-${decision}-${decided.Id}`,
        Accent: decision === 'reject' ? 'awaiting' : 'live',
        Title: t(`approvals.done.${decision}`, { environment: decided.Environment }),
      });
      onDone();
    });
  };

  const locked = action.Countdown.Remaining > 0;
  const label = locked
    ? t('throttle.retryIn', { time: formatClock(action.Countdown.Remaining) })
    : action.Busy
      ? t(`approvals.decide.${decision}.busy`)
      : t(`approvals.decide.${decision}.submit`);

  return (
    <form
      noValidate
      className="grid gap-5"
      onSubmit={(event) => {
        event.preventDefault();
        HandleSubmit();
      }}
    >
      <DecisionSummary approval={approval} />
      <p className="m-0 text-body-sm text-secondary">{t(`approvals.decide.${decision}.about`)}</p>
      {Problem ? <InlineAlert accent="failed" title={Problem} /> : null}
      {action.Notice ? <InlineAlert accent="failed" title={action.Notice} /> : null}
      <div className="grid gap-1.5">
        <Field label={bypass ? t('approvals.decide.reason') : t('approvals.decide.comment')}>
          <Textarea
            rows={3}
            disabled={action.Busy}
            value={Text}
            onChange={(event) => {
              setText(event.target.value);
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{bypass ? t('approvals.decide.reason.hint') : t('approvals.decide.comment.hint')}</small>
      </div>
      <div className="grid gap-3 border-t border-line pt-4">
        <p className="m-0 text-body-sm text-secondary">{t('approvals.factor.about')}</p>
        <FactorFields methods={methods} draft={action.Draft} onChange={action.SetDraft} disabled={action.Busy} />
        <DecisionButton decision={decision} label={label} disabled={action.Busy || locked} />
      </div>
    </form>
  );
};

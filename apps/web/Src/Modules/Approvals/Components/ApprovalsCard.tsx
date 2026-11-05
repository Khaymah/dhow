import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Card } from '@/Core/Ui/Card';
import { Rows, Row } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TApproval } from '@/Modules/Approvals/Api/ApprovalsApi';
import { ApprovalState } from '@/Modules/Approvals/Components/ApprovalState';

export interface IApprovalsCardProps {
  approvals: readonly TApproval[] | undefined;
}

const FACTORS = ['totp', 'recovery_code', 'passkey'] as const;

const ApprovalRow = ({ approval }: { approval: TApproval }) => {
  const t = useT();
  const time = useRelativeTime();
  const factor = FACTORS.find((kind) => kind === approval.FactorKind);
  const parts = [
    t('approvals.row.asked', { time: time.ago(approval.CreatedAt) }),
    approval.DecidedAt === undefined ? '' : t('approvals.row.decided', { time: time.ago(approval.DecidedAt) }),
    factor === undefined ? '' : t('approvals.row.factor', { factor: t(`approvals.factor.${factor}`) }),
  ].filter((part) => part !== '');

  return (
    <Row
      icon="shield"
      title={<bdi>{t('approvals.row.title', { environment: approval.Environment, round: approval.Round })}</bdi>}
      sub={
        <>
          <span className="block">{parts.join(' · ')}</span>
          {approval.Comment === undefined || approval.Comment === '' ? null : (
            <span className="block whitespace-pre-wrap break-words text-secondary">
              {t('approvals.row.comment')} <bdi>{approval.Comment}</bdi>
            </span>
          )}
          {approval.BypassReason === undefined || approval.BypassReason === '' ? null : (
            <span className="block whitespace-pre-wrap break-words text-secondary">
              {t('approvals.row.bypassReason')} <bdi>{approval.BypassReason}</bdi>
            </span>
          )}
        </>
      }
      end={<ApprovalState state={approval.State} />}
    />
  );
};

export const ApprovalsCard = ({ approvals }: IApprovalsCardProps) => {
  const t = useT();
  if (approvals === undefined || approvals.length === 0) return null;
  return (
    <Card title={t('approvals.card.title')} icon="shield" end={t('approvals.card.count', { count: approvals.length })}>
      <Rows label={t('approvals.card.title')}>
        {approvals.map((approval) => (
          <ApprovalRow key={approval.Id} approval={approval} />
        ))}
      </Rows>
    </Card>
  );
};

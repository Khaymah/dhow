import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import { LinkRow } from '@/Core/Ui/LinkRow';
import { Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TApproval } from '@/Modules/Approvals/Api/ApprovalsApi';
import { useMyApprovals } from '@/Modules/Approvals/Hooks/useApprovals';

export interface IMyApprovalsCardProps {
  organizationId: string;
  projectId?: string;
  hideWhenEmpty?: boolean;
  hrefFor: (approval: TApproval) => string;
  onOpen: (approval: TApproval) => void;
}

const shortRef = (ref: string): string => ref.replace(/^refs\/(heads|tags)\//, '');

export const MyApprovalsCard = ({ organizationId, projectId, hideWhenEmpty = false, hrefFor, onOpen }: IMyApprovalsCardProps) => {
  const t = useT();
  const time = useRelativeTime();
  const errorText = useApiErrorText();
  const approvals = useMyApprovals(organizationId);
  const items = (approvals.data ?? []).filter((approval) => projectId === undefined || approval.ProjectId === projectId);
  if (hideWhenEmpty && approvals.data !== undefined && items.length === 0) return null;

  return (
    <Card
      title={projectId === undefined ? t('approvals.page.list') : t('boards.approvals.title')}
      icon="shield"
      end={approvals.data === undefined ? null : t('approvals.page.count', { count: items.length })}
    >
      <CardStates
        state={cardStateOf(approvals, items.length === 0)}
        loadingLabel={t('approvals.page.loading')}
        errorTitle={t('approvals.page.error')}
        errorText={approvals.error ? errorText(approvals.error) : undefined}
        retryLabel={t('status.retry')}
        onRetry={() => {
          void approvals.refetch();
        }}
        emptyIcon="check"
        emptyText={projectId === undefined ? t('approvals.page.empty') : t('boards.approvals.empty')}
      >
        <Rows label={projectId === undefined ? t('approvals.page.list') : t('boards.approvals.title')}>
          {items.map((approval) => (
            <LinkRow
              key={approval.Id}
              icon="shield"
              href={hrefFor(approval)}
              onOpen={() => {
                onOpen(approval);
              }}
              title={
                <bdi>
                  {t('approvals.page.row', { environment: approval.Environment, number: approval.RunNumber })}
                </bdi>
              }
              sub={
                <>
                  <bdi dir="ltr" className="font-mono">
                    {shortRef(approval.Ref)}
                  </bdi>
                  {' · '}
                  {t('approvals.row.asked', { time: time.ago(approval.CreatedAt) })}
                  {' · '}
                  {t('approvals.gate.approval.expires', { time: time.full(approval.ExpiresAt) })}
                </>
              }
            />
          ))}
        </Rows>
      </CardStates>
    </Card>
  );
};

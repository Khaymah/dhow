import { Badge, type TBadgeTone } from '@/Core/Ui/Badge';
import { useT } from '@/I18n/useT';
import type { TApprovalState } from '@/Modules/Approvals/Api/ApprovalsApi';

const TONES: Record<TApprovalState, TBadgeTone> = {
  pending: 'warn',
  approved: 'ok',
  rejected: 'bad',
  expired: 'mute',
  superseded: 'mute',
  canceled: 'mute',
  bypassed: 'warn',
};

export const ApprovalState = ({ state }: { state: TApprovalState }) => {
  const t = useT();
  return <Badge tone={TONES[state]}>{t(`approvals.state.${state}`)}</Badge>;
};

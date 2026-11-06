import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { useT } from '@/I18n/useT';
import { revokeIngestToken, type TIngestToken } from '@/Modules/Ingest/Api/IngestApi';
import { useIngestRefresh } from '@/Modules/Ingest/Hooks/useIngest';

export interface IIngestTokenRevokeProps {
  organizationId: string;
  projectId: string;
  token: TIngestToken;
  onClose: () => void;
}

const RevokePanel = ({ organizationId, projectId, token, onClose, methods }: IIngestTokenRevokeProps & { methods: readonly TFactorMethod[] }) => {
  const t = useT();
  const refresh = useIngestRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const action = useFactorAction<TIngestToken>(methods, async (proof) =>
    revokeIngestToken(organizationId, projectId, token.Id, await proof()),
  );

  return (
    <ConfirmPanel
      danger
      question={t('ingest.revoke.question', { name: token.Name })}
      confirm={t('ingest.revoke.confirm')}
      icon="trash"
      methods={methods}
      action={action}
      onCancel={() => {
        action.Reset();
        onClose();
      }}
      onConfirm={() => {
        void action.Run().then(async (revoked) => {
          if (revoked === null) return;
          await refresh.Changed();
          Push({ Id: `ingest-token-revoked-${revoked.Id}`, Accent: 'live', Title: t('ingest.revoked', { name: revoked.Name }) });
          onClose();
        });
      }}
    />
  );
};

export const IngestTokenRevoke = (props: IIngestTokenRevokeProps) => (
  <li className="border-t border-line">
    <FactorGate>{(methods) => <RevokePanel {...props} methods={methods} />}</FactorGate>
  </li>
);

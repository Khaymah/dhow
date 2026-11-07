import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { useT } from '@/I18n/useT';
import { acknowledgeIngestTokenReach, type TIngestToken } from '@/Modules/Ingest/Api/IngestApi';
import { useIngestRefresh } from '@/Modules/Ingest/Hooks/useIngest';

export interface IIngestTokenAcknowledgeProps {
  organizationId: string;
  projectId: string;
  token: TIngestToken;
  onClose: () => void;
}

const AcknowledgePanel = ({
  organizationId,
  projectId,
  token,
  onClose,
  methods,
}: IIngestTokenAcknowledgeProps & { methods: readonly TFactorMethod[] }) => {
  const t = useT();
  const refresh = useIngestRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const action = useFactorAction<TIngestToken>(methods, async (proof) =>
    acknowledgeIngestTokenReach(organizationId, projectId, token.Id, await proof()),
  );

  return (
    <ConfirmPanel
      question={t('ingest.ack.question', { name: token.Name, environments: (token.UnacknowledgedReach ?? []).join(', ') })}
      confirm={t('ingest.ack.confirm')}
      icon="check"
      methods={methods}
      action={action}
      onCancel={() => {
        action.Reset();
        onClose();
      }}
      onConfirm={() => {
        void action.Run().then(async (confirmed) => {
          if (confirmed === null) return;
          await refresh.Changed();
          Push({ Id: `ingest-token-acknowledged-${confirmed.Id}`, Accent: 'live', Title: t('ingest.ack.done', { name: confirmed.Name }) });
          onClose();
        });
      }}
    />
  );
};

export const IngestTokenAcknowledge = (props: IIngestTokenAcknowledgeProps) => (
  <li className="border-t border-line">
    <FactorGate>{(methods) => <AcknowledgePanel {...props} methods={methods} />}</FactorGate>
  </li>
);

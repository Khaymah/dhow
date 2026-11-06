import type { TCurrentFactor, TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { useT } from '@/I18n/useT';

export interface ISecretFactorPanelProps {
  run: (factor: TCurrentFactor) => Promise<unknown>;
  removal?: boolean;
  onDone: () => void;
  onCancel: () => void;
}

const Panel = ({ methods, run, removal = false, onDone, onCancel }: ISecretFactorPanelProps & { methods: readonly TFactorMethod[] }) => {
  const t = useT();
  const action = useFactorAction<boolean>(methods, async (proof) => {
    await run(await proof());
    return true;
  });

  return (
    <ConfirmPanel
      question={t(removal ? 'secrets.factor.remove.question' : 'secrets.factor.question')}
      confirm={t(removal ? 'secrets.factor.remove.confirm' : 'secrets.factor.confirm')}
      icon="check"
      methods={methods}
      action={action}
      onCancel={() => {
        action.Reset();
        onCancel();
      }}
      onConfirm={() => {
        void action.Run().then((done) => {
          if (done === true) onDone();
        });
      }}
    />
  );
};

export const SecretFactorPanel = (props: ISecretFactorPanelProps) => (
  <FactorGate>{(methods) => <Panel {...props} methods={methods} />}</FactorGate>
);

import type { ReactNode } from 'react';

import type { TCurrentFactor, TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import type { TIconName } from '@/Core/Ui/Icons';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { FactorGate } from '@/Core/Ui/FactorGate';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';

export interface ITargetFactorPanelProps {
  question: string;
  confirm: string;
  icon: TIconName;
  danger?: boolean;
  disabled?: boolean;
  children?: ReactNode;
  run: (factor: TCurrentFactor) => Promise<TTarget>;
  onDone: (target: TTarget) => void;
  onCancel: () => void;
}

const Panel = ({ methods, run, onDone, onCancel, ...rest }: ITargetFactorPanelProps & { methods: readonly TFactorMethod[] }) => {
  const action = useFactorAction<TTarget>(methods, async (proof) => run(await proof()));

  return (
    <ConfirmPanel
      {...rest}
      methods={methods}
      action={action}
      onCancel={() => {
        action.Reset();
        onCancel();
      }}
      onConfirm={() => {
        void action.Run().then((target) => {
          if (target !== null) onDone(target);
        });
      }}
    />
  );
};

export const TargetFactorPanel = (props: ITargetFactorPanelProps) => (
  <FactorGate>{(methods) => <Panel {...props} methods={methods} />}</FactorGate>
);

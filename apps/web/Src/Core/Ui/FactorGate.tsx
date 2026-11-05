import type { ReactNode } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorMethods, useMfaStatus } from '@/Core/Auth/useMfaStatus';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';

export interface IFactorGateProps {
  children: (methods: readonly TFactorMethod[]) => ReactNode;
}

export const FactorGate = ({ children }: IFactorGateProps) => {
  const t = useT();
  const status = useMfaStatus();
  const methods = useFactorMethods();

  if (status.isError) {
    return (
      <div className="grid justify-items-start gap-2">
        <InlineAlert accent="failed" title={t('environments.factor.error')} />
        <Button
          size="sm"
          onClick={() => {
            void status.refetch();
          }}
        >
          {t('status.retry')}
        </Button>
      </div>
    );
  }
  if (methods === undefined) return <Skeleton label={t('ui.loading')} lines={3} />;
  return <>{children(methods)}</>;
};

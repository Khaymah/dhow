import { useState } from 'react';

import { Button } from '@/Core/Ui/Button';
import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';
import { Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { addTargetHost, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { HostItem } from '@/Modules/Environments/Components/HostItem';
import { TargetFactorPanel } from '@/Modules/Environments/Components/TargetFactorPanel';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';
import { DEFAULT_PORT, hostProblem, MAX_HOSTS } from '@/Modules/Environments/Types/TargetDraft';

export interface ITargetHostsPanelProps {
  scope: ITargetScope;
  target: TTarget;
  canManage: boolean;
}

const AddHost = ({ scope, target, onClose }: Omit<ITargetHostsPanelProps, 'canManage'> & { onClose: () => void }) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const [Address, setAddress] = useState('');
  const [Port, setPort] = useState(DEFAULT_PORT);
  const problem = hostProblem({ Host: Address, Port });

  return (
    <div className="grid gap-3 border-t border-line pt-3">
      <div className="flex min-w-0 items-end gap-2 px-card">
        <div className="min-w-0 flex-1">
          <Field label={t('targets.form.host')}>
            <Input
              compact
              dir="ltr"
              className="font-mono"
              maxLength={253}
              autoComplete="off"
              spellCheck={false}
              value={Address}
              onChange={(event) => {
                setAddress(event.target.value);
              }}
            />
          </Field>
        </div>
        <div className="w-24 flex-none">
          <Field label={t('targets.form.port')}>
            <Input
              compact
              dir="ltr"
              inputMode="numeric"
              className="font-mono"
              maxLength={5}
              autoComplete="off"
              value={Port}
              onChange={(event) => {
                setPort(event.target.value);
              }}
            />
          </Field>
        </div>
      </div>
      <TargetFactorPanel
        question={t('targets.host.add.question')}
        confirm={t('targets.host.add.confirm')}
        icon="plus"
        disabled={problem !== null}
        run={(factor) => addTargetHost(scope, target.Name, target.Version, { Host: Address.trim(), Port: Number(Port) }, factor)}
        onDone={() => {
          refresh.Changed();
          onClose();
        }}
        onCancel={onClose}
      />
    </div>
  );
};

export const TargetHostsPanel = ({ scope, target, canManage }: ITargetHostsPanelProps) => {
  const t = useT();
  const [Adding, setAdding] = useState(false);

  return (
    <div className="grid gap-3">
      <Rows label={t('targets.hosts.title')}>
        {target.Hosts.map((host) => (
          <HostItem key={host.Id} scope={scope} target={target} host={host} canManage={canManage} />
        ))}
      </Rows>
      {canManage && !Adding ? (
        <div className="px-card pb-card">
          <Button
            size="sm"
            iconStart="plus"
            disabled={target.Hosts.length >= MAX_HOSTS}
            onClick={() => {
              setAdding(true);
            }}
          >
            {t('targets.host.add')}
          </Button>
        </div>
      ) : null}
      {canManage && Adding ? (
        <AddHost
          scope={scope}
          target={target}
          onClose={() => {
            setAdding(false);
          }}
        />
      ) : null}
    </div>
  );
};

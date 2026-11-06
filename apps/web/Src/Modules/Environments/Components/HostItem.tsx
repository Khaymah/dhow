import { useState } from 'react';

import { useToastStore } from '@/Core/Store/useToastStore';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import type { TIconName } from '@/Core/Ui/Icons';
import { Input } from '@/Core/Ui/Input';
import { Field } from '@/Core/Ui/Field';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { removeTargetHost, reorderTargetHosts, updateTargetHost, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { HostKeyFlow } from '@/Modules/Environments/Components/HostKeyFlow';
import { TargetFactorPanel } from '@/Modules/Environments/Components/TargetFactorPanel';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';
import { hostProblem } from '@/Modules/Environments/Types/TargetDraft';

type THost = TTarget['Hosts'][number];
type TOpen = 'edit' | 'remove' | 'up' | 'down';

interface IHostAction {
  Open: TOpen;
  Icon: TIconName;
  Label: 'targets.host.moveUp' | 'targets.host.moveDown' | 'targets.host.edit' | 'targets.host.remove';
  Disabled: (target: TTarget, host: THost) => boolean;
}

const ACTIONS: readonly IHostAction[] = [
  { Open: 'up', Icon: 'swap', Label: 'targets.host.moveUp', Disabled: (_, host) => host.Position === 1 },
  { Open: 'down', Icon: 'swap', Label: 'targets.host.moveDown', Disabled: (target, host) => host.Position === target.Hosts.length },
  { Open: 'edit', Icon: 'settings', Label: 'targets.host.edit', Disabled: () => false },
  { Open: 'remove', Icon: 'trash', Label: 'targets.host.remove', Disabled: (target) => target.Hosts.length <= 1 },
];

export interface IHostItemProps {
  scope: ITargetScope;
  target: TTarget;
  host: THost;
  canManage: boolean;
}

const orderedWith = (target: TTarget, host: THost, step: -1 | 1): string[] => {
  const ids = target.Hosts.map((item) => item.Id);
  const at = ids.indexOf(host.Id);
  const to = at + step;
  if (at < 0 || to < 0 || to >= ids.length) return ids;
  const moved = ids[to];
  const mine = ids[at];
  if (moved === undefined || mine === undefined) return ids;
  ids[at] = moved;
  ids[to] = mine;
  return ids;
};

const EditAddress = ({ scope, target, host, onClose }: IHostItemProps & { onClose: () => void }) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const [Address, setAddress] = useState(host.Host);
  const [Port, setPort] = useState(String(host.Port));
  const problem = hostProblem({ Host: Address, Port });

  return (
    <div className="grid gap-3 px-card pb-3">
      <div className="flex min-w-0 items-end gap-2">
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
      {problem ? <small className="text-body-sm text-bad-fg">{t(problem)}</small> : null}
      <TargetFactorPanel
        question={t('targets.host.edit.question')}
        confirm={t('targets.host.edit.confirm')}
        icon="check"
        disabled={problem !== null}
        run={(factor) =>
          updateTargetHost(scope, target.Name, host.Id, target.Version, { Host: Address.trim(), Port: Number(Port) }, factor)
        }
        onDone={() => {
          refresh.Changed();
          onClose();
        }}
        onCancel={onClose}
      />
    </div>
  );
};

const HostPanels = ({ open, onClose, ...props }: IHostItemProps & { open: TOpen; onClose: () => void }) => {
  const t = useT();
  const { scope, target, host } = props;
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const Push = useToastStore((state) => state.Push);
  const done = (title: string) => () => {
    refresh.Changed();
    Push({ Id: `host-${open}-${host.Id}`, Accent: 'live', Title: title });
    onClose();
  };

  if (open === 'edit') return <EditAddress {...props} onClose={onClose} />;
  if (open === 'remove') {
    return (
      <TargetFactorPanel
        danger
        question={t('targets.host.remove.question', { host: host.Host })}
        confirm={t('targets.host.remove.confirm')}
        icon="trash"
        run={(factor) => removeTargetHost(scope, target.Name, host.Id, target.Version, factor)}
        onDone={done(t('targets.host.removed', { host: host.Host }))}
        onCancel={onClose}
      />
    );
  }
  return (
    <TargetFactorPanel
      question={t('targets.host.move.question', { host: host.Host })}
      confirm={t('targets.host.move.confirm')}
      icon="swap"
      run={(factor) => reorderTargetHosts(scope, target.Name, target.Version, orderedWith(target, host, open === 'up' ? -1 : 1), factor)}
      onDone={done(t('targets.host.moved', { host: host.Host }))}
      onCancel={onClose}
    />
  );
};

export const HostItem = (props: IHostItemProps) => {
  const t = useT();
  const { target, host, canManage } = props;
  const [Open, setOpen] = useState<TOpen | null>(null);
  const close = () => {
    setOpen(null);
  };

  return (
    <li className="min-w-0 border-t border-line first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-3 px-card py-3">
        <span className="w-6 flex-none text-body-sm text-tertiary">{host.Position}</span>
        <bdi dir="ltr" className="min-w-0 flex-1 font-mono text-label">
          {host.Host}:{host.Port}
        </bdi>
        {host.KeyPinned ? (
          <Badge tone="ok" icon="check">
            {t('targets.hostkey.badge.pinned')}
          </Badge>
        ) : (
          <Badge tone="warn" icon="alert">
            {t('targets.hostkey.badge.unpinned')}
          </Badge>
        )}
        {canManage ? (
          <span className="flex flex-none flex-wrap items-center gap-1">
            {ACTIONS.map((action) => (
              <Button
                key={action.Open}
                size="sm"
                variant="ghost"
                iconStart={action.Icon}
                disabled={action.Disabled(target, host)}
                onClick={() => {
                  setOpen(action.Open);
                }}
              >
                {t(action.Label)}
              </Button>
            ))}
          </span>
        ) : null}
      </div>
      {Open ? <HostPanels {...props} open={Open} onClose={close} /> : null}
      <HostKeyFlow {...props} />
    </li>
  );
};

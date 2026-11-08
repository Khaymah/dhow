import { useState } from 'react';

import type { TCurrentFactor, TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { FactorGate } from '@/Core/Ui/FactorGate';
import type { TIconName } from '@/Core/Ui/Icons';
import { useT } from '@/I18n/useT';
import { removeChannel, updateChannel, type TCreatedNotificationChannel, type TNotificationChannel } from '@/Modules/Notifications/Api/NotificationsApi';
import { SecretCreated } from '@/Modules/Notifications/Components/SecretCreated';
import { useNotificationsRefresh } from '@/Modules/Notifications/Hooks/useNotifications';

export type TChannelChange = 'remove' | 'enable' | 'rotate';

export interface IChannelConfirmProps {
  organizationId: string;
  projectId: string;
  channel: TNotificationChannel;
  change: TChannelChange;
  onClose: () => void;
}

const ICONS: Record<TChannelChange, TIconName> = { remove: 'trash', enable: 'check', rotate: 'swap' };

const run = async (
  { organizationId, projectId, channel, change }: IChannelConfirmProps,
  proof: () => Promise<TCurrentFactor>,
): Promise<TCreatedNotificationChannel | TNotificationChannel> => {
  const factor = await proof();
  if (change === 'remove') return removeChannel(organizationId, projectId, channel.Id, factor);
  const base = { Name: channel.Name, Locale: channel.Locale, Version: channel.Version };
  const input = change === 'enable' ? { ...base, Enable: true } : { ...base, RotateSecret: true };
  return updateChannel(organizationId, projectId, channel.Id, input, factor);
};

const Panel = (props: IChannelConfirmProps & { methods: readonly TFactorMethod[] }) => {
  const t = useT();
  const { organizationId, projectId, channel, change, onClose, methods } = props;
  const refresh = useNotificationsRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const [Secret, setSecret] = useState<string | null>(null);
  const action = useFactorAction<TCreatedNotificationChannel | TNotificationChannel>(methods, (proof) => run(props, proof));

  if (Secret !== null) {
    return (
      <div className="px-card pb-card">
        <SecretCreated
          secret={Secret}
          onDone={() => {
            setSecret(null);
            onClose();
          }}
        />
      </div>
    );
  }

  return (
    <ConfirmPanel
      danger={change === 'remove'}
      question={t(`notifications.${change}.question`, { name: channel.Name })}
      confirm={t(`notifications.${change}.confirm`)}
      icon={ICONS[change]}
      methods={methods}
      action={action}
      onCancel={() => {
        action.Reset();
        onClose();
      }}
      onConfirm={() => {
        void action.Run().then(async (done) => {
          if (done === null) return;
          await refresh.Changed();
          if ('Secret' in done && done.Secret !== undefined) {
            setSecret(done.Secret);
            return;
          }
          Push({ Id: `notification-${change}-${channel.Id}`, Accent: 'live', Title: t(`notifications.${change}.done`, { name: channel.Name }) });
          onClose();
        });
      }}
    />
  );
};

export const ChannelConfirm = (props: IChannelConfirmProps) => (
  <FactorGate>{(methods) => <Panel {...props} methods={methods} />}</FactorGate>
);

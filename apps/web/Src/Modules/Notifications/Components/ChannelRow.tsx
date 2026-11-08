import { useState } from 'react';

import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Icon } from '@/Core/Ui/Icon';
import type { TIconName } from '@/Core/Ui/Icons';
import { useT } from '@/I18n/useT';
import type { TNotificationChannel, TNotificationKind, INotificationRule } from '@/Modules/Notifications/Api/NotificationsApi';
import { ChannelConfirm, type TChannelChange } from '@/Modules/Notifications/Components/ChannelConfirm';
import { RulesEditor } from '@/Modules/Notifications/Components/RulesEditor';
import { TestButton } from '@/Modules/Notifications/Components/TestButton';
import { kindLabel } from '@/Modules/Notifications/Types/NotificationsForm';

export interface IChannelRowProps {
  organizationId: string;
  projectId: string;
  channel: TNotificationChannel;
  rule: INotificationRule | undefined;
  manage: boolean;
  writable: boolean;
}

type TPanel = 'rules' | TChannelChange;

const KIND_ICONS: Record<TNotificationKind, TIconName> = {
  webhook: 'terminal',
  email: 'mail',
  inapp: 'bell',
  slack: 'layers',
  discord: 'users',
  telegram: 'phone',
};

const StateBadge = ({ channel }: { channel: TNotificationChannel }) => {
  const t = useT();
  if (channel.Enabled) return <Badge tone="ok">{t('notifications.state.active')}</Badge>;
  const reason = channel.DisabledReason === 'failures' ? 'notifications.state.failures' : 'notifications.state.creatorRemoved';
  return (
    <Badge tone="warn" icon="alert">
      {t(reason)}
    </Badge>
  );
};

export const ChannelRow = ({ organizationId, projectId, channel, rule, manage, writable }: IChannelRowProps) => {
  const t = useT();
  const [Panel, setPanel] = useState<TPanel | null>(null);
  const toggle = (next: TPanel) => {
    setPanel((current) => (current === next ? null : next));
  };
  const close = () => {
    setPanel(null);
  };
  const facts = [t(kindLabel(channel.Kind)), channel.Host, t(`language.${channel.Locale}`)].filter((fact): fact is string => fact !== undefined && fact !== '');

  return (
    <li className="min-w-0 border-t border-line first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 px-card py-3">
        <Icon name={KIND_ICONS[channel.Kind]} size={18} className="text-secondary" />
        <span className="grid min-w-0 flex-1 basis-48 gap-0.5">
          <span className="min-w-0 text-label">{channel.Name}</span>
          <small className="min-w-0 text-body-sm text-tertiary">{facts.join(' · ')}</small>
        </span>
        <span className="ms-auto flex max-w-full flex-wrap items-center gap-1.5">
          <StateBadge channel={channel} />
          <Button
            size="sm"
            variant="ghost"
            aria-expanded={Panel === 'rules'}
            aria-label={t('notifications.rules.open', { name: channel.Name })}
            onClick={() => {
              toggle('rules');
            }}
          >
            {t('notifications.rules.button')}
          </Button>
          {manage && writable ? <TestButton organizationId={organizationId} projectId={projectId} channel={channel} /> : null}
          {manage && writable && !channel.Enabled ? (
            <Button
              size="sm"
              aria-label={t('notifications.enable.aria', { name: channel.Name })}
              onClick={() => {
                toggle('enable');
              }}
            >
              {t('notifications.enable.button')}
            </Button>
          ) : null}
          {manage && writable && channel.Kind === 'webhook' ? (
            <Button
              size="sm"
              variant="ghost"
              aria-label={t('notifications.rotate.aria', { name: channel.Name })}
              onClick={() => {
                toggle('rotate');
              }}
            >
              {t('notifications.rotate.button')}
            </Button>
          ) : null}
          {manage && writable ? (
            <Button
              size="sm"
              variant="danger"
              iconStart="trash"
              aria-label={t('notifications.remove.aria', { name: channel.Name })}
              onClick={() => {
                toggle('remove');
              }}
            >
              {t('notifications.remove.button')}
            </Button>
          ) : null}
        </span>
      </div>
      {Panel === 'rules' ? (
        <RulesEditor organizationId={organizationId} projectId={projectId} channel={channel} rule={rule} writable={manage && writable} onClose={close} />
      ) : null}
      {Panel !== null && Panel !== 'rules' ? (
        <ChannelConfirm organizationId={organizationId} projectId={projectId} channel={channel} change={Panel} onClose={close} />
      ) : null}
    </li>
  );
};

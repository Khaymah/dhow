import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Badge, type TBadgeTone } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { retryDelivery, type TDeliveryState, type TNotificationDelivery } from '@/Modules/Notifications/Api/NotificationsApi';
import { useNotificationsRefresh } from '@/Modules/Notifications/Hooks/useNotifications';
import { codeKey, stateKey } from '@/Modules/Notifications/Types/DeliveryCodes';
import { kindLabel, knownEventLabel } from '@/Modules/Notifications/Types/NotificationsForm';

export interface IDeliveryRowProps {
  organizationId: string;
  projectId: string;
  delivery: TNotificationDelivery;
}

const TONES: Record<TDeliveryState, TBadgeTone> = { pending: 'queue', sending: 'info', sent: 'ok', dead: 'bad' };

export const DeliveryRow = ({ organizationId, projectId, delivery }: IDeliveryRowProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const time = useRelativeTime();
  const refresh = useNotificationsRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const [Busy, setBusy] = useState(false);
  const [Notice, setNotice] = useState<string | null>(null);
  const known = codeKey(delivery.LastError);
  const eventText = t(knownEventLabel(delivery.Event));
  const last = delivery.LastAttemptAt ?? delivery.CreatedAt;

  const HandleRetry = () => {
    setBusy(true);
    setNotice(null);
    retryDelivery(organizationId, projectId, delivery.Id)
      .then(async () => {
        await refresh.Changed();
        Push({ Id: `notification-retry-${delivery.Id}`, Accent: 'live', Title: t('notifications.deliveries.retried', { name: delivery.ChannelName }) });
      })
      .catch((error: unknown) => {
        setNotice(errorText(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <li className="min-w-0 border-t border-line first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 px-card py-3">
        <span className="grid min-w-0 flex-1 basis-56 gap-0.5">
          <span className="min-w-0 text-label">{delivery.ChannelName}</span>
          <small className="min-w-0 text-body-sm text-tertiary">
            {[eventText, t(kindLabel(delivery.ChannelKind)), time.ago(last)].join(' · ')}
          </small>
          {delivery.LastError !== '' ? (
            <small className="min-w-0 text-body-sm text-secondary">
              {known === null ? <bdi dir="ltr">{delivery.LastError}</bdi> : t(known)}
              {delivery.LastStatus !== undefined ? <bdi dir="ltr"> {delivery.LastStatus}</bdi> : null}
            </small>
          ) : null}
        </span>
        <span className="ms-auto flex max-w-full flex-wrap items-center gap-1.5">
          <small className="text-body-sm text-tertiary">{t('notifications.deliveries.attempts', { count: delivery.Attempts })}</small>
          <Badge tone={TONES[delivery.State]}>{t(stateKey(delivery.State))}</Badge>
          {delivery.State === 'dead' ? (
            <Button
              size="sm"
              iconStart="swap"
              disabled={Busy}
              aria-label={t('notifications.deliveries.retry.aria', { name: delivery.ChannelName })}
              onClick={HandleRetry}
            >
              {t('notifications.deliveries.retry')}
            </Button>
          ) : null}
        </span>
      </div>
      {Notice ? (
        <div className="px-card pb-3">
          <InlineAlert focus accent="failed" title={Notice} />
        </div>
      ) : null}
    </li>
  );
};

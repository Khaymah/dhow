import { useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import { formatClock } from '@/Core/Format/TimeFormat';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useCountdown } from '@/Core/Hooks/useCountdown';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { testChannel, type TNotificationChannel } from '@/Modules/Notifications/Api/NotificationsApi';
import { useNotificationsRefresh } from '@/Modules/Notifications/Hooks/useNotifications';

export interface ITestButtonProps {
  organizationId: string;
  projectId: string;
  channel: TNotificationChannel;
}

export const TestButton = ({ organizationId, projectId, channel }: ITestButtonProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const refresh = useNotificationsRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const Countdown = useCountdown();
  const [Busy, setBusy] = useState(false);
  const [Notice, setNotice] = useState<string | null>(null);
  const locked = Countdown.Remaining > 0;

  const HandleTest = () => {
    setBusy(true);
    setNotice(null);
    testChannel(organizationId, projectId, channel.Id)
      .then(async () => {
        await refresh.Changed();
        Push({ Id: `notification-test-${channel.Id}`, Accent: 'live', Title: t('notifications.test.queued', { name: channel.Name }) });
      })
      .catch((error: unknown) => {
        if (isApiError(error) && error.RetryAfterSeconds !== null) Countdown.Start(error.RetryAfterSeconds);
        setNotice(errorText(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <>
      <Button size="sm" iconStart="play" disabled={Busy || locked || !channel.Enabled} onClick={HandleTest}>
        {locked ? t('throttle.retryIn', { time: formatClock(Countdown.Remaining) }) : t('notifications.test.send')}
      </Button>
      {Notice ? (
        <div className="basis-full">
          <InlineAlert focus accent="failed" title={t('notifications.test.failed', { name: channel.Name })}>
            {Notice}
          </InlineAlert>
        </div>
      ) : null}
    </>
  );
};

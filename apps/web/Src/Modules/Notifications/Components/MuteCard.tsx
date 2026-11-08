import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { CheckList } from '@/Core/Ui/CheckList';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { useMute, useSetMute } from '@/Modules/Notifications/Hooks/useNotifications';

export const MuteCard = () => {
  const t = useT();
  const errorText = useApiErrorText();
  const mute = useMute();
  const save = useSetMute();

  return (
    <Card title={t('notifications.mute.title')} icon="bell" padded>
      <div className="grid gap-3">
        <p className="m-0 text-body-sm text-secondary">{t('notifications.mute.lead')}</p>
        {mute.isPending ? <Skeleton label={t('ui.loading')} lines={1} /> : null}
        {mute.isError ? (
          <div className="grid justify-items-start gap-3">
            <InlineAlert accent="failed" title={t('notifications.mute.error')}>
              {errorText(mute.error)}
            </InlineAlert>
            <Button
              size="sm"
              onClick={() => {
                void mute.refetch();
              }}
            >
              {t('projects.retry')}
            </Button>
          </div>
        ) : null}
        {mute.isSuccess ? (
          <CheckList
            label={t('notifications.mute.title')}
            disabled={save.isPending}
            values={mute.data ? ['muted'] : []}
            items={[{ value: 'muted', title: t('notifications.mute.option'), about: t('notifications.mute.about') }]}
            onChange={(values) => {
              save.mutate(values.includes('muted'));
            }}
          />
        ) : null}
        {save.isError ? <InlineAlert focus accent="failed" title={errorText(save.error)} /> : null}
      </div>
    </Card>
  );
};

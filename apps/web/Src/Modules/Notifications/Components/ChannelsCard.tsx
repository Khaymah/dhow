import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { ChannelRow } from '@/Modules/Notifications/Components/ChannelRow';
import { useChannels, useRules } from '@/Modules/Notifications/Hooks/useNotifications';

export interface IChannelsCardProps {
  organizationId: string;
  projectId: string;
  manage: boolean;
  writable: boolean;
  onNew: () => void;
}

export const ChannelsCard = ({ organizationId, projectId, manage, writable, onNew }: IChannelsCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const channels = useChannels(organizationId, projectId);
  const rules = useRules(organizationId, projectId);

  if (channels.isPending || rules.isPending) return <Skeleton label={t('notifications.channels.loading')} lines={3} />;
  if (channels.isError || rules.isError) {
    const failed = channels.isError ? channels.error : rules.error;
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('notifications.channels.error')}>
          {errorText(failed)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void channels.refetch();
            void rules.refetch();
          }}
        >
          {t('projects.retry')}
        </Button>
      </div>
    );
  }
  const { Items, Meta } = channels.data;

  return (
    <Card title={t('notifications.channels.title')} icon="bell" end={Meta.Total > 0 ? String(Meta.Total) : undefined}>
      <p className="m-0 px-card pb-2 text-body-sm text-secondary">{t('notifications.channels.lead')}</p>
      {Items.length === 0 ? <p className="m-0 px-card pb-3 text-body-sm text-secondary">{t('notifications.channels.empty')}</p> : null}
      <ul aria-label={t('notifications.channels.title')} className="m-0 grid list-none p-0">
        {Items.map((channel) => (
          <ChannelRow
            key={channel.Id}
            organizationId={organizationId}
            projectId={projectId}
            channel={channel}
            rule={rules.data.find((rule) => rule.ChannelId === channel.Id)}
            manage={manage}
            writable={writable}
          />
        ))}
      </ul>
      {manage && writable ? (
        <div className="p-card pt-2">
          <Button className="w-full" iconStart="plus" onClick={onNew}>
            {t('notifications.channels.new')}
          </Button>
        </div>
      ) : null}
    </Card>
  );
};

import { useRouter } from '@tanstack/react-router';

import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import type { TInboxNotification } from '@/Modules/Notifications/Api/InboxApi';
import { detailsOf, linkOf, titleOf } from '@/Modules/Notifications/Types/InboxView';

export interface IInboxRowProps {
  item: TInboxNotification;
  busy: boolean;
  onRead: (item: TInboxNotification) => void;
}

export const InboxRow = ({ item, busy, onRead }: IInboxRowProps) => {
  const t = useT();
  const time = useRelativeTime();
  const router = useRouter();
  const link = linkOf(item);
  const title = t(titleOf(item));
  const details = detailsOf(item);

  const HandleOpen = () => {
    if (link === null) return;
    if (!item.Read) onRead(item);
    router.history.push(link);
  };

  return (
    <li className="min-w-0 border-t border-line first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 px-card py-3">
        <span className="grid min-w-0 flex-1 basis-56 gap-1">
          <span className="min-w-0 text-label">
            {link === null ? (
              title
            ) : (
              <a href={link} onClick={followInApp(HandleOpen)} className="text-primary underline-offset-4 hover:underline">
                {title}
              </a>
            )}
          </span>
          {details.length > 0 ? (
            <small className="min-w-0 text-body-sm text-secondary">
              {details.map((detail, index) => (
                <span key={detail.Label}>
                  {index > 0 ? ' · ' : ''}
                  {t(detail.Label)}: {detail.Key === null ? <bdi dir="auto">{detail.Value}</bdi> : t(detail.Key)}
                </span>
              ))}
            </small>
          ) : null}
          <small className="min-w-0 text-body-sm text-tertiary">
            <time dateTime={item.CreatedAt} title={time.full(item.CreatedAt)}>
              {time.ago(item.CreatedAt)}
            </time>
          </small>
        </span>
        <span className="ms-auto flex max-w-full flex-wrap items-center gap-1.5">
          {item.Read ? null : <Badge tone="info">{t('notifications.inbox.unread')}</Badge>}
          {item.Read ? null : (
            <Button
              size="sm"
              variant="ghost"
              iconStart="check"
              disabled={busy}
              aria-label={t('notifications.inbox.markRead.aria', { title })}
              onClick={() => {
                onRead(item);
              }}
            >
              {t('notifications.inbox.markRead')}
            </Button>
          )}
        </span>
      </div>
    </li>
  );
};

import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Pager } from '@/Core/Ui/Pager';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { INBOX_PAGE_SIZE, type TInboxNotification } from '@/Modules/Notifications/Api/InboxApi';
import { InboxRow } from '@/Modules/Notifications/Components/InboxRow';
import { useInbox, useMarkAllRead, useMarkRead } from '@/Modules/Notifications/Hooks/useInbox';

const FILTERS = ['all', 'unread'] as const;

export const InboxList = () => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Page, setPage] = useState(1);
  const [UnreadOnly, setUnreadOnly] = useState(false);
  const inbox = useInbox(Page, UnreadOnly);
  const markRead = useMarkRead();
  const markAll = useMarkAllRead();

  if (inbox.isPending) return <Skeleton label={t('notifications.inbox.loading')} lines={4} />;
  if (inbox.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('notifications.inbox.error')}>
          {errorText(inbox.error)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void inbox.refetch();
          }}
        >
          {t('projects.retry')}
        </Button>
      </div>
    );
  }
  const { Items, Meta } = inbox.data;
  const failure = markRead.error ?? markAll.error;

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="group" aria-label={t('notifications.inbox.filter')} className="flex flex-wrap gap-2">
          {FILTERS.map((filter) => (
            <Button
              key={filter}
              size="sm"
              variant={(filter === 'unread') === UnreadOnly ? 'primary' : 'secondary'}
              aria-pressed={(filter === 'unread') === UnreadOnly}
              onClick={() => {
                setUnreadOnly(filter === 'unread');
                setPage(1);
              }}
            >
              {t(`notifications.inbox.filter.${filter}`)}
            </Button>
          ))}
        </div>
        <Button
          size="sm"
          iconStart="check"
          disabled={markAll.isPending}
          onClick={() => {
            markAll.mutate();
          }}
        >
          {t('notifications.inbox.markAll')}
        </Button>
      </div>
      {failure ? <InlineAlert focus accent="failed" title={errorText(failure)} /> : null}
      <Card>
        {Items.length === 0 ? (
          <EmptyState icon="bell" text={UnreadOnly ? t('notifications.inbox.emptyUnread') : t('notifications.empty')} />
        ) : (
          <ul aria-label={t('notifications.title')} className="m-0 grid list-none p-0">
            {Items.map((item) => (
              <InboxRow
                key={item.Id}
                item={item}
                busy={markRead.isPending}
                onRead={(target: TInboxNotification) => {
                  markRead.mutate(target.Id);
                }}
              />
            ))}
          </ul>
        )}
      </Card>
      <Pager page={Page} limit={INBOX_PAGE_SIZE} total={Meta.Total} busy={inbox.isFetching} onPage={setPage} />
    </div>
  );
};

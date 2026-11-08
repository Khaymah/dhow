import { ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import { INBOX_KEY } from '@/Modules/Notifications/Api/InboxKey';

export type TInboxNotification = components['schemas']['InboxNotification'];

export const INBOX_PAGE_SIZE = 20;
export const inboxListKey = (unreadOnly: boolean, page: number) => [...INBOX_KEY, 'list', unreadOnly, page] as const;

export const listInbox = (page: number, unreadOnly: boolean): Promise<IPage<TInboxNotification>> =>
  requestPage(() =>
    ApiClient.GET('/me/notifications', { params: { query: { Page: page, Limit: INBOX_PAGE_SIZE, Unread: unreadOnly ? true : undefined } } }),
  );

export const markInboxRead = (notificationId: string): Promise<TInboxNotification> =>
  request(() => ApiClient.POST('/me/notifications/{notificationId}/read', { params: { path: { notificationId } } }));

export const markAllInboxRead = (): Promise<{ Marked: number }> => request(() => ApiClient.POST('/me/notifications/read-all'));

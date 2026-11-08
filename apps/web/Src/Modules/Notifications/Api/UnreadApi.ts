import { ApiClient, request } from '@/Core/Api/Client';

export const UNREAD_KEY = ['inbox', 'unread'] as const;
export const UNREAD_REFRESH_MS = 60_000;

export const getUnreadCount = (): Promise<number> =>
  request(() => ApiClient.GET('/me/notifications/unread-count')).then((count) => count.Unread);

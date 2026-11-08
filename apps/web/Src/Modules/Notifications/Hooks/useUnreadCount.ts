import { useQuery } from '@tanstack/react-query';

import { getUnreadCount, UNREAD_KEY, UNREAD_REFRESH_MS } from '@/Modules/Notifications/Api/UnreadApi';

export const useUnreadCount = (enabled: boolean): number => {
  const count = useQuery<number>({
    queryKey: UNREAD_KEY,
    queryFn: getUnreadCount,
    enabled,
    refetchInterval: UNREAD_REFRESH_MS,
  });
  return count.data ?? 0;
};

import { useQuery } from '@tanstack/react-query';

import { useCurrentMembership } from '@/Core/Auth/useMe';
import { useOrgEventsMode } from '@/Core/Hooks/useOrgEvents';
import { getUnreadCount, UNREAD_KEY, UNREAD_REFRESH_MS } from '@/Modules/Notifications/Api/UnreadApi';

export const useUnreadCount = (enabled: boolean): number => {
  const live = useOrgEventsMode(useCurrentMembership()?.OrganizationId) === 'live';
  const count = useQuery<number>({
    queryKey: UNREAD_KEY,
    queryFn: getUnreadCount,
    enabled,
    refetchInterval: live ? false : UNREAD_REFRESH_MS,
  });
  return count.data ?? 0;
};

import { keepPreviousData, useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IPage } from '@/Core/Api/Client';
import { INBOX_KEY } from '@/Modules/Notifications/Api/InboxKey';
import {
  inboxListKey,
  listInbox,
  markAllInboxRead,
  markInboxRead,
  type TInboxNotification,
} from '@/Modules/Notifications/Api/InboxApi';

export const useInbox = (page: number, unreadOnly: boolean): UseQueryResult<IPage<TInboxNotification>, ApiError> =>
  useQuery<IPage<TInboxNotification>, ApiError>({
    queryKey: inboxListKey(unreadOnly, page),
    queryFn: () => listInbox(page, unreadOnly),
    placeholderData: keepPreviousData,
  });

export const useMarkRead = () => {
  const client = useQueryClient();
  return useMutation<TInboxNotification, ApiError, string>({
    mutationFn: markInboxRead,
    onSuccess: () => client.invalidateQueries({ queryKey: INBOX_KEY }),
  });
};

export const useMarkAllRead = () => {
  const client = useQueryClient();
  return useMutation<{ Marked: number }, ApiError>({
    mutationFn: markAllInboxRead,
    onSuccess: () => client.invalidateQueries({ queryKey: INBOX_KEY }),
  });
};

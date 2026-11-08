import { keepPreviousData, useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IPage } from '@/Core/Api/Client';
import {
  channelsKey,
  deliveriesKey,
  getMute,
  listChannels,
  listDeliveries,
  listRules,
  MUTE_KEY,
  notificationsKey,
  rulesKey,
  setMute,
  type TDeliveryState,
  type TNotificationChannel,
  type TNotificationDelivery,
  type INotificationRule,
} from '@/Modules/Notifications/Api/NotificationsApi';

export const useChannels = (organizationId: string, projectId: string): UseQueryResult<IPage<TNotificationChannel>, ApiError> =>
  useQuery<IPage<TNotificationChannel>, ApiError>({
    queryKey: channelsKey(organizationId, projectId),
    queryFn: () => listChannels(organizationId, projectId),
  });

export const useRules = (organizationId: string, projectId: string): UseQueryResult<INotificationRule[], ApiError> =>
  useQuery<INotificationRule[], ApiError>({
    queryKey: rulesKey(organizationId, projectId),
    queryFn: () => listRules(organizationId, projectId),
  });

export const useDeliveries = (
  organizationId: string,
  projectId: string,
  page: number,
  state: TDeliveryState | undefined,
): UseQueryResult<IPage<TNotificationDelivery>, ApiError> =>
  useQuery<IPage<TNotificationDelivery>, ApiError>({
    queryKey: [...deliveriesKey(organizationId, projectId), page, state ?? 'all'],
    queryFn: () => listDeliveries(organizationId, projectId, page, state),
    placeholderData: keepPreviousData,
  });

export const useNotificationsRefresh = (organizationId: string, projectId: string) => {
  const client = useQueryClient();
  return {
    Changed: () => client.invalidateQueries({ queryKey: notificationsKey(organizationId, projectId) }),
  };
};

export const useMute = (): UseQueryResult<boolean, ApiError> =>
  useQuery<boolean, ApiError>({ queryKey: MUTE_KEY, queryFn: getMute });

export const useSetMute = () => {
  const client = useQueryClient();
  return useMutation<boolean, ApiError, boolean>({
    mutationFn: setMute,
    onSuccess: (muted) => {
      client.setQueryData(MUTE_KEY, muted);
    },
  });
};

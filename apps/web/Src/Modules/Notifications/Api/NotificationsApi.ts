import { ApiClient, request, requestPage, type IPage } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';

type TSchemas = components['schemas'];

export type TNotificationChannel = TSchemas['NotificationChannel'];
export type TNotificationKind = TSchemas['NotificationChannelKind'];
export type TNotificationRuleInput = TSchemas['NotificationRuleInput'];
export type TNotificationDelivery = TSchemas['NotificationDelivery'];
export type TDeliveryState = TSchemas['NotificationDeliveryState'];
export type TCreatedNotificationChannel = TSchemas['CreatedNotificationChannel'];
export type TNotificationEvent = TNotificationRuleInput['Events'][number];
export type TNotificationStatus = TNotificationRuleInput['Statuses'][number];
export type TNotificationRole = NonNullable<TNotificationRuleInput['Roles']>[number];

export interface INotificationRule {
  ChannelId: string;
  Events: TNotificationEvent[];
  Environments: string[];
  Statuses: TNotificationStatus[];
  Roles: string[];
  Version: number;
}

export const CHANNELS_PAGE_SIZE = 50;
export const DELIVERIES_PAGE_SIZE = 20;

export const notificationsKey = (organizationId: string, projectId: string) => ['notifications', organizationId, projectId] as const;
export const channelsKey = (organizationId: string, projectId: string) => [...notificationsKey(organizationId, projectId), 'channels'] as const;
export const rulesKey = (organizationId: string, projectId: string) => [...notificationsKey(organizationId, projectId), 'rules'] as const;
export const deliveriesKey = (organizationId: string, projectId: string) =>
  [...notificationsKey(organizationId, projectId), 'deliveries'] as const;
export const MUTE_KEY = ['notifications', 'mute'] as const;

export interface ICreateChannel {
  Kind: TNotificationKind;
  Name: string;
  Locale: 'ar' | 'en';
  Url?: string;
  BotToken?: string;
  ChatId?: string;
}

export interface IUpdateChannel {
  Name: string;
  Locale: 'ar' | 'en';
  Version: number;
  Enable?: boolean;
  RotateSecret?: boolean;
}

export const listChannels = (organizationId: string, projectId: string): Promise<IPage<TNotificationChannel>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/notification-channels', {
      params: { path: { id: organizationId, projectId }, query: { Page: 1, Limit: CHANNELS_PAGE_SIZE } },
    }),
  );

export const createChannel = (
  organizationId: string,
  projectId: string,
  input: ICreateChannel,
  factor: TCurrentFactor,
): Promise<TCreatedNotificationChannel> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/notification-channels', {
      params: { path: { id: organizationId, projectId } },
      body: { ...input, CurrentFactor: factor },
    }),
  );

export const updateChannel = (
  organizationId: string,
  projectId: string,
  channelId: string,
  input: IUpdateChannel,
  factor: TCurrentFactor,
): Promise<TCreatedNotificationChannel> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/notification-channels/{channelId}', {
      params: { path: { id: organizationId, projectId, channelId } },
      body: { ...input, CurrentFactor: factor },
    }),
  );

export const removeChannel = (
  organizationId: string,
  projectId: string,
  channelId: string,
  factor: TCurrentFactor,
): Promise<TNotificationChannel> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/notification-channels/{channelId}/remove', {
      params: { path: { id: organizationId, projectId, channelId } },
      body: { CurrentFactor: factor },
    }),
  );

export const testChannel = (organizationId: string, projectId: string, channelId: string): Promise<{ EventId: string }> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/notification-channels/{channelId}/test', {
      params: { path: { id: organizationId, projectId, channelId } },
    }),
  );

export const listRules = (organizationId: string, projectId: string): Promise<INotificationRule[]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/notification-rules', {
      params: { path: { id: organizationId, projectId } },
    }),
  );

export const putRule = (organizationId: string, projectId: string, rule: TNotificationRuleInput): Promise<INotificationRule[]> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/notification-rules', {
      params: { path: { id: organizationId, projectId } },
      body: { Rules: [rule] },
    }),
  );

export const listDeliveries = (
  organizationId: string,
  projectId: string,
  page: number,
  state: TDeliveryState | undefined,
): Promise<IPage<TNotificationDelivery>> =>
  requestPage(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/notification-deliveries', {
      params: { path: { id: organizationId, projectId }, query: { Page: page, Limit: DELIVERIES_PAGE_SIZE, State: state } },
    }),
  );

export const retryDelivery = (organizationId: string, projectId: string, deliveryId: string): Promise<{ Retried?: boolean }> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/notification-deliveries/{deliveryId}/retry', {
      params: { path: { id: organizationId, projectId, deliveryId } },
    }),
  );

export const getMute = (): Promise<boolean> =>
  request(() => ApiClient.GET('/me/notification-mute')).then((mute) => mute.Muted);

export const setMute = (muted: boolean): Promise<boolean> =>
  request(() => ApiClient.PUT('/me/notification-mute', { body: { Muted: muted } })).then((mute) => mute.Muted);

import type { TMessageKey } from '@/I18n/Messages';
import type { TDeliveryState } from '@/Modules/Notifications/Api/NotificationsApi';

const CODES: Record<string, TMessageKey> = {
  blocked: 'notifications.code.blocked',
  redirect: 'notifications.code.redirect',
  timeout: 'notifications.code.timeout',
  connect: 'notifications.code.connect',
  http_status: 'notifications.code.http_status',
  config: 'notifications.code.config',
  channel_removed: 'notifications.code.channel_removed',
  channel_disabled: 'notifications.code.channel_disabled',
  mail_disabled: 'notifications.code.mail_disabled',
  mail_send: 'notifications.code.mail_send',
  no_recipient: 'notifications.code.no_recipient',
  lease_expired: 'notifications.code.lease_expired',
};

export const DELIVERY_STATES: readonly TDeliveryState[] = ['pending', 'sending', 'sent', 'dead'];

export const codeKey = (code: string): TMessageKey | null => (Object.hasOwn(CODES, code) ? (CODES[code] ?? null) : null);

export const stateKey = (state: TDeliveryState): TMessageKey => `notifications.deliveries.state.${state}`;

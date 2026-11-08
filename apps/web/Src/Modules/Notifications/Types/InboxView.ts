import type { TMessageKey } from '@/I18n/Messages';
import type { TInboxNotification } from '@/Modules/Notifications/Api/InboxApi';

const TITLES = new Map<string, TMessageKey>([
  ['inbox.deployment.finished.succeeded', 'notifications.inbox.deployment.finished.succeeded'],
  ['inbox.deployment.finished.failed', 'notifications.inbox.deployment.finished.failed'],
  ['inbox.deployment.finished.canceled', 'notifications.inbox.deployment.finished.canceled'],
  ['inbox.deployment.finished.rolled_back', 'notifications.inbox.deployment.finished.rolled_back'],
  ['inbox.deployment.finished.rollback_failed', 'notifications.inbox.deployment.finished.rollback_failed'],
  ['inbox.deployment.approval.requested', 'notifications.inbox.deployment.approval.requested'],
  ['inbox.run.finished', 'notifications.inbox.run.finished'],
  ['inbox.runner.offline', 'notifications.inbox.runner.offline'],
  ['inbox.account.new_device_login', 'notifications.inbox.account.new_device_login'],
  ['inbox.account.new_device_login.self', 'notifications.inbox.account.new_device_login.self'],
  ['inbox.account.second_factor_changed', 'notifications.inbox.account.second_factor_changed'],
  ['inbox.account.second_factor_changed.self', 'notifications.inbox.account.second_factor_changed.self'],
  ['inbox.member.removed', 'notifications.inbox.member.removed'],
  ['inbox.member.role_changed', 'notifications.inbox.member.role_changed'],
  ['inbox.member.role_changed.self', 'notifications.inbox.member.role_changed.self'],
  ['inbox.notification.test', 'notifications.inbox.notification.test'],
]);

const CHANGES = new Map<string, TMessageKey>([
  ['totp_enabled', 'notifications.inbox.change.totp_enabled'],
  ['totp_disabled', 'notifications.inbox.change.totp_disabled'],
  ['passkey_added', 'notifications.inbox.change.passkey_added'],
  ['passkey_removed', 'notifications.inbox.change.passkey_removed'],
  ['recovery_codes_regenerated', 'notifications.inbox.change.recovery_codes_regenerated'],
]);

const ROLES = new Map<string, TMessageKey>([
  ['owner', 'role.owner'],
  ['admin', 'role.admin'],
  ['maintainer', 'role.maintainer'],
  ['developer', 'role.developer'],
  ['viewer', 'role.viewer'],
]);

const FIELDS: readonly (readonly [string, TMessageKey])[] = [
  ['project', 'notifications.inbox.field.project'],
  ['environment', 'notifications.inbox.field.environment'],
  ['ref', 'notifications.inbox.field.ref'],
  ['run', 'notifications.inbox.field.run'],
  ['runner', 'notifications.inbox.field.runner'],
  ['browser', 'notifications.inbox.field.browser'],
  ['release', 'notifications.inbox.field.release'],
];

const SAFE_LINK = /^\/(?!\/)[A-Za-z0-9._~!$&'()*+,;=:@%/?#-]*$/;
const LINK_MAX = 512;

export interface IInboxDetail {
  Label: TMessageKey;
  Value: string;
  Key: TMessageKey | null;
}

export const titleOf = (item: TInboxNotification): TMessageKey => TITLES.get(item.TitleKey) ?? 'notifications.inbox.unknown';

export const linkOf = (item: TInboxNotification): string | null => {
  const link = item.Link;
  return link !== undefined && link.length <= LINK_MAX && SAFE_LINK.test(link) ? link : null;
};

export const detailsOf = (item: TInboxNotification): IInboxDetail[] => {
  const out: IInboxDetail[] = [];
  for (const [name, label] of FIELDS) {
    const value = Object.hasOwn(item.Params, name) ? item.Params[name] : undefined;
    if (value !== undefined && value !== '') out.push({ Label: label, Value: value, Key: null });
  }
  const change = Object.hasOwn(item.Params, 'change') ? CHANGES.get(item.Params.change ?? '') : undefined;
  if (change !== undefined) out.push({ Label: 'notifications.inbox.field.change', Value: '', Key: change });
  const role = item.Event === 'member.role_changed' && Object.hasOwn(item.Params, 'reason') ? ROLES.get(item.Params.reason ?? '') : undefined;
  if (role !== undefined) out.push({ Label: 'notifications.inbox.field.role', Value: '', Key: role });
  return out;
};

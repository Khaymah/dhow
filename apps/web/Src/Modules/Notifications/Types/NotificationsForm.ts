import type { TMessageKey } from '@/I18n/Messages';
import type {
  ICreateChannel,
  TNotificationEvent,
  TNotificationKind,
  TNotificationRole,
  TNotificationRuleInput,
  TNotificationStatus,
} from '@/Modules/Notifications/Api/NotificationsApi';

export const KINDS: readonly TNotificationKind[] = ['webhook', 'email', 'inapp', 'slack', 'discord', 'telegram'];
export const EVENTS: readonly TNotificationEvent[] = [
  'deployment.finished',
  'deployment.approval.requested',
  'run.finished',
  'runner.offline',
  'account.new_device_login',
  'account.second_factor_changed',
  'member.removed',
  'member.role_changed',
];
export const STATUSES: readonly TNotificationStatus[] = ['succeeded', 'failed', 'canceled', 'rolled_back', 'rollback_failed', 'awaiting_approval'];
export const RULE_ROLES: readonly TNotificationRole[] = ['owner', 'admin', 'maintainer', 'developer', 'viewer'];

const BY_ROLE: readonly TNotificationKind[] = ['email', 'inapp'];
const URL_KINDS: readonly TNotificationKind[] = ['webhook', 'slack', 'discord'];
const NAME_MAX = 64;
const ENVIRONMENT = /^[a-z0-9][a-z0-9_-]{0,63}$/;

export const sendsByRole = (kind: TNotificationKind): boolean => BY_ROLE.includes(kind);
export const needsUrl = (kind: TNotificationKind): boolean => URL_KINDS.includes(kind);
export const needsBot = (kind: TNotificationKind): boolean => kind === 'telegram';

export const kindLabel = (kind: TNotificationKind): TMessageKey => `notifications.kind.${kind}`;
export const eventLabel = (event: TNotificationEvent): TMessageKey => `notifications.event.${event}`;
export const knownEventLabel = (event: string): TMessageKey => {
  const known = EVENTS.find((candidate) => candidate === event);
  return known === undefined ? 'notifications.event.test' : eventLabel(known);
};
export const statusLabel = (status: TNotificationStatus): TMessageKey => `notifications.status.${status}`;

export interface IChannelDraft {
  Kind: TNotificationKind;
  Name: string;
  Locale: 'ar' | 'en';
  Url: string;
  BotToken: string;
  ChatId: string;
}

export const EMPTY_CHANNEL_DRAFT: IChannelDraft = { Kind: 'webhook', Name: '', Locale: 'ar', Url: '', BotToken: '', ChatId: '' };

export const draftProblem = (draft: IChannelDraft): TMessageKey | null => {
  const name = draft.Name.trim();
  if (name === '' || Array.from(name).length > NAME_MAX) return 'notifications.form.nameInvalid';
  if (needsUrl(draft.Kind) && !draft.Url.trim().toLowerCase().startsWith('https://')) return 'notifications.form.urlInvalid';
  if (needsBot(draft.Kind) && draft.BotToken.trim() === '') return 'notifications.form.botInvalid';
  if (needsBot(draft.Kind) && draft.ChatId.trim() === '') return 'notifications.form.chatInvalid';
  return null;
};

export const createInputOf = (draft: IChannelDraft): ICreateChannel => {
  const input: ICreateChannel = { Kind: draft.Kind, Name: draft.Name.trim(), Locale: draft.Locale };
  if (needsUrl(draft.Kind)) input.Url = draft.Url.trim();
  if (needsBot(draft.Kind)) {
    input.BotToken = draft.BotToken.trim();
    input.ChatId = draft.ChatId.trim();
  }
  return input;
};

export interface IRuleDraft {
  Events: TNotificationEvent[];
  Statuses: TNotificationStatus[];
  Roles: TNotificationRole[];
  Environments: string;
}

export const environmentsOf = (text: string): string[] => [
  ...new Set(
    text
      .split(/[\s,]+/)
      .map((name) => name.trim())
      .filter((name) => name !== ''),
  ),
];

export const ruleProblem = (kind: TNotificationKind, draft: IRuleDraft): TMessageKey | null => {
  if (draft.Events.length === 0) return 'notifications.rules.eventsEmpty';
  if (draft.Statuses.length === 0) return 'notifications.rules.statusesEmpty';
  if (sendsByRole(kind) && draft.Roles.length === 0) return 'notifications.rules.rolesEmpty';
  if (environmentsOf(draft.Environments).some((name) => !ENVIRONMENT.test(name))) return 'notifications.rules.environmentsInvalid';
  return null;
};

export const ruleInputOf = (channelId: string, kind: TNotificationKind, draft: IRuleDraft): TNotificationRuleInput => ({
  ChannelId: channelId,
  Events: draft.Events,
  Statuses: draft.Statuses,
  Environments: environmentsOf(draft.Environments),
  Roles: sendsByRole(kind) ? draft.Roles : [],
});

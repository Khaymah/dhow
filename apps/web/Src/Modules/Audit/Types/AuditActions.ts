import type { TIconName } from '@/Core/Ui/Icons';
import type { TMessageKey } from '@/I18n/Messages';
import { isMessageKey } from '@/I18n/Translate';

interface IAuditAction {
  Icon: TIconName;
  Bad: boolean;
}

export const AUDIT_ACTIONS: Readonly<Record<string, IAuditAction>> = {
  'auth.login': { Icon: 'check', Bad: false },
  'auth.login_failed': { Icon: 'alert', Bad: true },
  'auth.logout': { Icon: 'out', Bad: false },
  'auth.password_changed': { Icon: 'key', Bad: false },
  'auth.session_revoked': { Icon: 'monitor', Bad: false },
  'auth.sessions_revoked_others': { Icon: 'monitor', Bad: false },
  'auth.mfa_pending': { Icon: 'shield', Bad: false },
  'auth.mfa_failed': { Icon: 'alert', Bad: true },
  'auth.recovery_code_used': { Icon: 'key', Bad: false },
  'auth.totp_enabled': { Icon: 'shield', Bad: false },
  'auth.totp_disabled': { Icon: 'shield', Bad: true },
  'auth.recovery_codes_regenerated': { Icon: 'key', Bad: false },
  'auth.passkey_added': { Icon: 'key', Bad: false },
  'auth.passkey_renamed': { Icon: 'key', Bad: false },
  'auth.passkey_removed': { Icon: 'key', Bad: true },
  'auth.passkey_sign_count_regressed': { Icon: 'alert', Bad: true },
  'auth.oidc_login_failed': { Icon: 'alert', Bad: true },
  'auth.identity_linked': { Icon: 'user', Bad: false },
  'auth.identity_unlinked': { Icon: 'user', Bad: true },
  'auth.api_token_created': { Icon: 'key', Bad: false },
  'auth.api_token_revoked': { Icon: 'key', Bad: true },
  'identity.organization.created': { Icon: 'building', Bad: false },
  'identity.organization.updated': { Icon: 'building', Bad: false },
  'identity.organization.ownership_transferred': { Icon: 'swap', Bad: true },
  'identity.organization.second_factor_changed': { Icon: 'shield', Bad: false },
  'identity.member.left': { Icon: 'out', Bad: false },
  'identity.member.role_changed': { Icon: 'user', Bad: false },
  'identity.member.removed': { Icon: 'user', Bad: true },
  'identity.member.mfa_reset': { Icon: 'shield', Bad: true },
  'identity.team.created': { Icon: 'users', Bad: false },
  'identity.team.updated': { Icon: 'users', Bad: false },
  'identity.team.deleted': { Icon: 'users', Bad: true },
  'identity.team.member_added': { Icon: 'users', Bad: false },
  'identity.team.member_removed': { Icon: 'users', Bad: false },
  'identity.team.role_binding_added': { Icon: 'users', Bad: false },
  'identity.team.role_binding_changed': { Icon: 'users', Bad: false },
  'identity.team.role_binding_removed': { Icon: 'users', Bad: false },
  'identity.invitation.created': { Icon: 'mail', Bad: false },
  'identity.invitation.resent': { Icon: 'mail', Bad: false },
  'identity.invitation.revoked': { Icon: 'mail', Bad: false },
  'identity.invitation.accepted': { Icon: 'mail', Bad: false },
  'runner.registered': { Icon: 'server', Bad: false },
  'runner.updated': { Icon: 'server', Bad: false },
  'runner.revoked': { Icon: 'server', Bad: true },
  'runner.disabled_clone_suspected': { Icon: 'alert', Bad: true },
  'runner.keys_rotated': { Icon: 'key', Bad: false },
  'runner.registration_token_created': { Icon: 'key', Bad: false },
  'runner.registration_token_revoked': { Icon: 'key', Bad: false },
  'job.created': { Icon: 'queue', Bad: false },
  'job.cancel_requested': { Icon: 'queue', Bad: false },
  'project.created': { Icon: 'layers', Bad: false },
  'project.updated': { Icon: 'layers', Bad: false },
  'project.archived': { Icon: 'layers', Bad: true },
  'project.repository_linked': { Icon: 'layers', Bad: false },
  'project.repository_updated': { Icon: 'layers', Bad: false },
  'project.host_key_confirmed': { Icon: 'key', Bad: false },
  'project.deploy_key_rotated': { Icon: 'key', Bad: false },
  'project.deploy_key_confirmed': { Icon: 'key', Bad: false },
  'project.webhook_secret_rotated': { Icon: 'key', Bad: false },
  'project.repository_unlinked': { Icon: 'trash', Bad: true },
  'project.run_created': { Icon: 'layers', Bad: false },
  'project.run_started': { Icon: 'layers', Bad: false },
  'project.run_cancel_requested': { Icon: 'layers', Bad: false },
  'project.run_rerun_requested': { Icon: 'layers', Bad: false },
  'project.run_approved': { Icon: 'shield', Bad: false },
  'environment.created': { Icon: 'shield', Bad: false },
  'environment.updated': { Icon: 'shield', Bad: false },
  'environment.removed': { Icon: 'trash', Bad: true },
  'environment.paused': { Icon: 'stop', Bad: true },
  'environment.resumed': { Icon: 'play', Bad: false },
  'system.instance.setupCompleted': { Icon: 'shield', Bad: false },
  'system.settings.changed': { Icon: 'settings', Bad: false },
  'system.keys.activated': { Icon: 'key', Bad: false },
  'system.keys.exported': { Icon: 'key', Bad: false },
  'system.keys.discarded': { Icon: 'alert', Bad: true },
  'system.keys.rekeyed': { Icon: 'key', Bad: false },
  'system.keys.rekey_started': { Icon: 'key', Bad: false },
  'system.keys.rekey_failed': { Icon: 'alert', Bad: true },
};

const GROUPS = ['auth', 'identity', 'runner', 'job', 'project', 'system'] as const;

export type TAuditGroup = (typeof GROUPS)[number] | 'other';

const GROUP_ICONS: Record<TAuditGroup, TIconName> = {
  auth: 'shield',
  identity: 'building',
  runner: 'server',
  job: 'queue',
  project: 'layers',
  system: 'settings',
  other: 'file',
};

export const actionGroup = (action: string): TAuditGroup => {
  const prefix = action.split('.', 1)[0];
  return GROUPS.find((group) => group === prefix) ?? 'other';
};

export const actionKey = (action: string): TMessageKey | null => {
  const key = `audit.action.${action}`;
  return isMessageKey(key) ? key : null;
};

export const groupKey = (action: string): TMessageKey => `audit.group.${actionGroup(action)}`;

export const actionIcon = (action: string): TIconName => AUDIT_ACTIONS[action]?.Icon ?? GROUP_ICONS[actionGroup(action)];

export const isBadAction = (action: string): boolean => AUDIT_ACTIONS[action]?.Bad ?? false;

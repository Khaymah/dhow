import { anyone, instanceOwner, member, type TGate } from '@/App/Navigation/Gates';
import { canReadSecrets, type TRole } from '@/Core/Auth/AuthTypes';
import type { TIconName } from '@/Core/Ui/Icons';
import type { TMessageKey } from '@/I18n/Messages';
import { canReadIngested } from '@/Modules/Ingest/Types/IngestAccess';
import { canManageNotifications } from '@/Modules/Notifications/Types/NotificationsAccess';
import { canOnProject } from '@/Modules/Projects/Types/ProjectAccess';

export type TSettingsPage = 'org' | 'project' | 'projectNotifications' | 'system' | 'security' | 'preferences';

export type TSettingId =
  | 'org.general'
  | 'org.security'
  | 'org.danger'
  | 'project.general'
  | 'project.repository'
  | 'project.ingest'
  | 'project.registry'
  | 'project.danger'
  | 'project.notifications'
  | 'project.channels'
  | 'project.deliveries'
  | 'security.password'
  | 'security.secondFactor'
  | 'security.sessions'
  | 'security.tokens'
  | 'preferences.language'
  | 'preferences.notifications'
  | 'preferences.appearance'
  | 'system.mail'
  | 'system.providers'
  | 'system.organizations'
  | 'system.passwords'
  | 'system.rateLimits'
  | 'system.logs';

export interface ISettingEntry {
  id: TSettingId;
  page: TSettingsPage;
  anchor: string | null;
  label: TMessageKey;
  icon: TIconName;
  allow: TGate;
  activeOnly?: boolean;
}

export interface ISettingsPageInfo {
  label: TMessageKey;
  icon: TIconName;
  scoped: boolean;
}

export interface ISettingsContext {
  role: TRole | undefined;
  owner: boolean;
  projectActive: boolean;
}

export const SETTINGS_PAGES: Record<TSettingsPage, ISettingsPageInfo> = {
  org: { label: 'shell.nav.orgSettings', icon: 'building', scoped: false },
  project: { label: 'projects.settings.title', icon: 'settings', scoped: true },
  projectNotifications: { label: 'settings.page.projectNotifications', icon: 'bell', scoped: true },
  system: { label: 'shell.nav.systemSettings', icon: 'settings', scoped: false },
  security: { label: 'shell.nav.security', icon: 'shield', scoped: false },
  preferences: { label: 'shell.nav.preferences', icon: 'settings', scoped: false },
};

export const SETTINGS: readonly ISettingEntry[] = [
  { id: 'org.general', page: 'org', anchor: 'general', label: 'orgSettings.group.general', icon: 'building', allow: member },
  { id: 'org.security', page: 'org', anchor: 'security', label: 'orgSettings.group.security', icon: 'shield', allow: member },
  { id: 'org.danger', page: 'org', anchor: 'danger', label: 'orgSettings.group.danger', icon: 'alert', allow: member },
  { id: 'project.general', page: 'project', anchor: 'general', label: 'projects.group.general', icon: 'settings', allow: member },
  { id: 'project.repository', page: 'project', anchor: 'repository', label: 'projects.group.repository', icon: 'layers', allow: member },
  { id: 'project.ingest', page: 'project', anchor: 'ingest', label: 'ingest.settings.title', icon: 'download', allow: canReadIngested },
  { id: 'project.registry', page: 'project', anchor: 'registry', label: 'secrets.group.registry', icon: 'server', allow: canReadSecrets },
  {
    id: 'project.danger',
    page: 'project',
    anchor: 'danger',
    label: 'projects.danger.title',
    icon: 'alert',
    allow: (role) => canOnProject(role, 'project.manage'),
    activeOnly: true,
  },
  { id: 'project.notifications', page: 'projectNotifications', anchor: null, label: 'projects.nav.notifications', icon: 'bell', allow: member },
  { id: 'project.channels', page: 'projectNotifications', anchor: 'channels', label: 'notifications.channels.group', icon: 'bell', allow: member },
  { id: 'project.deliveries', page: 'projectNotifications', anchor: 'deliveries', label: 'notifications.deliveries.group', icon: 'file', allow: canManageNotifications },
  { id: 'security.password', page: 'security', anchor: 'password', label: 'security.group.password', icon: 'key', allow: anyone },
  { id: 'security.secondFactor', page: 'security', anchor: 'second-factor', label: 'security.group.secondFactor', icon: 'shield', allow: anyone },
  { id: 'security.sessions', page: 'security', anchor: 'sessions', label: 'security.group.sessions', icon: 'monitor', allow: anyone },
  { id: 'security.tokens', page: 'security', anchor: 'tokens', label: 'security.group.tokens', icon: 'key', allow: anyone },
  { id: 'preferences.language', page: 'preferences', anchor: 'language', label: 'preferences.language.title', icon: 'monitor', allow: anyone },
  { id: 'preferences.notifications', page: 'preferences', anchor: 'notifications', label: 'shell.nav.notifications', icon: 'bell', allow: anyone },
  { id: 'preferences.appearance', page: 'preferences', anchor: 'appearance', label: 'appearance.title', icon: 'layers', allow: anyone },
  { id: 'system.mail', page: 'system', anchor: 'mail', label: 'settings.mail.title', icon: 'mail', allow: instanceOwner },
  { id: 'system.providers', page: 'system', anchor: 'providers', label: 'settings.oidc.title', icon: 'user', allow: instanceOwner },
  { id: 'system.organizations', page: 'system', anchor: 'organizations', label: 'settings.organizations.title', icon: 'building', allow: instanceOwner },
  { id: 'system.passwords', page: 'system', anchor: 'passwords', label: 'settings.passwords.title', icon: 'key', allow: instanceOwner },
  { id: 'system.rateLimits', page: 'system', anchor: 'rate-limits', label: 'settings.rateLimits.title', icon: 'shield', allow: instanceOwner },
  { id: 'system.logs', page: 'system', anchor: 'logs', label: 'settings.logs.title', icon: 'file', allow: instanceOwner },
];

export const settingEntry = (id: TSettingId): ISettingEntry => {
  const found = SETTINGS.find((entry) => entry.id === id);
  if (found === undefined) throw new Error(`unknown setting ${id}`);
  return found;
};

export const settingsOfPage = (page: TSettingsPage): ISettingEntry[] => SETTINGS.filter((entry) => entry.page === page);

export const settingVisible = (entry: ISettingEntry, context: ISettingsContext): boolean =>
  entry.allow(context.role, context.owner) && (entry.activeOnly !== true || context.projectActive);

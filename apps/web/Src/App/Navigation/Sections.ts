import { anyone, instanceOwner, member, type TGate } from '@/App/Navigation/Gates';
import {
  canApproveDeployments,
  canManageMembers,
  canManageRunners,
  canManageTokens,
  canReadAudit,
  canReadMembers,
  canReadRunners,
  canReadSecrets,
  type TRole,
} from '@/Core/Auth/AuthTypes';
import type { TIconName } from '@/Core/Ui/Icons';
import type { TMessageKey } from '@/I18n/Messages';

export type TGroupId = 'run' | 'org' | 'watch' | 'account';

export type TSectionId =
  | 'home'
  | 'projects'
  | 'jobs'
  | 'approvals'
  | 'runners'
  | 'runnerTokens'
  | 'members'
  | 'teams'
  | 'invitations'
  | 'apiKeys'
  | 'secrets'
  | 'orgSettings'
  | 'audit'
  | 'system'
  | 'systemSettings'
  | 'about'
  | 'profile'
  | 'security'
  | 'preferences'
  | 'organizations'
  | 'notifications';

export type TSectionPath =
  | '/'
  | '/projects'
  | '/jobs'
  | '/approvals'
  | '/runners'
  | '/runners/tokens'
  | '/org/members'
  | '/org/teams'
  | '/org/invitations'
  | '/org/api-keys'
  | '/org/secrets'
  | '/org/settings'
  | '/audit'
  | '/system'
  | '/system/settings'
  | '/about'
  | '/account/profile'
  | '/account/security'
  | '/account/preferences'
  | '/organizations'
  | '/notifications';

export interface ISection {
  id: TSectionId;
  path: TSectionPath;
  label: TMessageKey;
  icon: TIconName;
  group?: TGroupId;
  parent?: TSectionId;
  menu: boolean;
  detail?: TMessageKey;
  detailAllow?: TGate;
  allow: TGate;
}

export const GROUP_LABELS: Record<TGroupId, TMessageKey> = {
  run: 'shell.group.run',
  org: 'shell.group.org',
  watch: 'shell.group.watch',
  account: 'shell.group.account',
};

export const MENU_GROUPS: readonly TGroupId[] = ['run', 'org', 'watch'];

export const SECTIONS: readonly ISection[] = [
  { id: 'home', path: '/', label: 'shell.nav.home', icon: 'home', menu: true, allow: anyone },
  {
    id: 'projects',
    path: '/projects',
    label: 'shell.nav.projects',
    icon: 'folder',
    group: 'run',
    menu: true,
    detail: 'shell.crumb.project',
    detailAllow: member,
    allow: member,
  },
  {
    id: 'jobs',
    path: '/jobs',
    label: 'shell.nav.jobs',
    icon: 'play',
    group: 'run',
    menu: true,
    detail: 'jobs.detail.title',
    detailAllow: member,
    allow: member,
  },
  {
    id: 'approvals',
    path: '/approvals',
    label: 'shell.nav.approvals',
    icon: 'check',
    group: 'run',
    menu: true,
    allow: canApproveDeployments,
  },
  {
    id: 'runners',
    path: '/runners',
    label: 'shell.nav.runners',
    icon: 'server',
    group: 'run',
    menu: true,
    detail: 'runners.detail.title',
    detailAllow: canReadRunners,
    allow: canReadRunners,
  },
  {
    id: 'runnerTokens',
    path: '/runners/tokens',
    label: 'shell.nav.runnerTokens',
    icon: 'key',
    group: 'run',
    parent: 'runners',
    menu: false,
    allow: canManageRunners,
  },
  {
    id: 'members',
    path: '/org/members',
    label: 'shell.nav.members',
    icon: 'users',
    group: 'org',
    menu: true,
    detail: 'shell.crumb.member',
    detailAllow: canReadMembers,
    allow: canReadMembers,
  },
  {
    id: 'teams',
    path: '/org/teams',
    label: 'shell.nav.teams',
    icon: 'layers',
    group: 'org',
    menu: true,
    detail: 'shell.crumb.team',
    detailAllow: canReadMembers,
    allow: canReadMembers,
  },
  { id: 'invitations', path: '/org/invitations', label: 'shell.nav.invitations', icon: 'mail', group: 'org', menu: true, allow: canManageMembers },
  { id: 'apiKeys', path: '/org/api-keys', label: 'shell.nav.apiKeys', icon: 'key', group: 'org', menu: true, allow: canManageTokens },
  { id: 'secrets', path: '/org/secrets', label: 'shell.nav.secrets', icon: 'shield', group: 'org', menu: true, allow: canReadSecrets },
  { id: 'orgSettings', path: '/org/settings', label: 'shell.nav.orgSettings', icon: 'settings', group: 'org', menu: true, allow: member },
  {
    id: 'audit',
    path: '/audit',
    label: 'shell.nav.audit',
    icon: 'file',
    group: 'watch',
    menu: true,
    detail: 'audit.detail.title',
    detailAllow: canReadAudit,
    allow: canReadAudit,
  },
  { id: 'system', path: '/system', label: 'shell.nav.system', icon: 'shield', group: 'watch', menu: true, detailAllow: anyone, allow: anyone },
  {
    id: 'systemSettings',
    path: '/system/settings',
    label: 'shell.nav.systemSettings',
    icon: 'settings',
    group: 'watch',
    menu: true,
    allow: instanceOwner,
  },
  { id: 'about', path: '/about', label: 'shell.nav.about', icon: 'info', group: 'watch', menu: true, allow: anyone },
  { id: 'profile', path: '/account/profile', label: 'shell.nav.profile', icon: 'user', group: 'account', menu: false, allow: anyone },
  { id: 'security', path: '/account/security', label: 'shell.nav.security', icon: 'shield', group: 'account', menu: false, allow: anyone },
  { id: 'preferences', path: '/account/preferences', label: 'shell.nav.preferences', icon: 'settings', group: 'account', menu: false, allow: anyone },
  { id: 'organizations', path: '/organizations', label: 'shell.nav.organizations', icon: 'building', group: 'org', menu: false, allow: anyone },
  { id: 'notifications', path: '/notifications', label: 'shell.nav.notifications', icon: 'bell', menu: false, allow: anyone },
];

const BY_ID = Object.fromEntries(SECTIONS.map((section) => [section.id, section])) as Record<TSectionId, ISection>;

export const sectionById = (id: TSectionId): ISection => BY_ID[id];

const trimmed = (pathname: string): string => (pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname);

export interface ISectionMatch {
  section: ISection;
  rest: string | null;
}

export const matchSection = (pathname: string): ISectionMatch | null => {
  const path = trimmed(pathname);
  let best: ISectionMatch | null = null;
  for (const section of SECTIONS) {
    if (path === section.path) return { section, rest: null };
    if (section.path !== '/' && path.startsWith(`${section.path}/`)) {
      if (best === null || section.path.length > best.section.path.length) {
        best = { section, rest: path.slice(section.path.length + 1) };
      }
    }
  }
  return best;
};

export const visibleSections = (role: TRole | undefined, group: TGroupId, owner = false): ISection[] =>
  SECTIONS.filter((section) => section.group === group && section.allow(role, owner));

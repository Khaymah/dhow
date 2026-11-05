import { useNavigate } from '@tanstack/react-router';

import type { TGate } from '@/App/Navigation/Gates';
import { SECTIONS, type TSectionId, type TSectionPath } from '@/App/Navigation/Sections';
import type { TSlideId } from '@/App/Panels';
import { canCreateProjects, canManageMembers, canManageRunners, canManageTokens } from '@/Core/Auth/AuthTypes';
import { useCurrentMembership, useMe } from '@/Core/Auth/useMe';
import { useSignOut } from '@/Core/Auth/useSignOut';
import type { ICommandItem } from '@/Core/Ui/CommandPalette';
import type { TIconName } from '@/Core/Ui/Icons';
import { otherLocale } from '@/I18n/Locale';
import type { TMessageKey } from '@/I18n/Messages';
import { useI18n } from '@/I18n/useI18n';

export interface ICommands {
  items: ICommandItem[];
  run: (item: ICommandItem) => void;
}

const PAGE_COMMANDS: Record<TSectionId, TMessageKey> = {
  home: 'palette.command.home',
  projects: 'palette.command.projects',
  jobs: 'palette.command.jobs',
  approvals: 'palette.command.approvals',
  runners: 'palette.command.runners',
  runnerTokens: 'palette.command.runnerTokens',
  members: 'palette.command.members',
  teams: 'palette.command.teams',
  invitations: 'palette.command.invitations',
  apiKeys: 'palette.command.organizationTokens',
  secrets: 'palette.command.secrets',
  orgSettings: 'palette.command.orgSettings',
  audit: 'palette.command.audit',
  system: 'palette.command.systemStatus',
  systemSettings: 'palette.command.systemSettings',
  profile: 'palette.command.account',
  security: 'palette.command.security',
  preferences: 'palette.command.preferences',
  organizations: 'palette.command.organizations',
  notifications: 'palette.command.notifications',
};

interface ICreateCommand {
  id: string;
  icon: TIconName;
  label: TMessageKey;
  path: TSectionPath;
  slide: TSlideId;
  allow?: TGate;
}

const CREATE_COMMANDS: readonly ICreateCommand[] = [
  { id: 'invite', icon: 'plus', label: 'palette.command.invite', path: '/org/members', slide: 'organization-invite', allow: canManageMembers },
  { id: 'team-new', icon: 'plus', label: 'palette.command.newTeam', path: '/org/teams', slide: 'team-new', allow: canManageMembers },
  {
    id: 'organization-token-new',
    icon: 'key',
    label: 'palette.command.newApiKey',
    path: '/org/api-keys',
    slide: 'organization-token-new',
    allow: canManageTokens,
  },
  { id: 'project-new', icon: 'folder', label: 'palette.command.newProject', path: '/projects', slide: 'project-new', allow: canCreateProjects },
  { id: 'runner-new', icon: 'server', label: 'palette.command.newRunner', path: '/runners', slide: 'runner-new', allow: canManageRunners },
  { id: 'password', icon: 'key', label: 'palette.command.password', path: '/account/security', slide: 'account-password' },
  { id: 'account-token-new', icon: 'key', label: 'palette.command.newAccountToken', path: '/account/profile', slide: 'account-token-new' },
  { id: 'theme-new', icon: 'plus', label: 'palette.command.createTheme', path: '/account/preferences', slide: 'theme-edit' },
  { id: 'organization-new', icon: 'building', label: 'palette.command.newOrganization', path: '/organizations', slide: 'organization-new' },
];

export const useCommandItems = (): ICommands => {
  const { Locale, SetLocale, t } = useI18n();
  const navigate = useNavigate();
  const { SignOut } = useSignOut();
  const role = useCurrentMembership()?.Role;
  const owner = useMe().data?.InstanceOwner === true;
  const target = otherLocale(Locale);
  const pages = SECTIONS.filter((section) => section.allow(role, owner));
  const creates = CREATE_COMMANDS.filter((command) => command.allow === undefined || command.allow(role));

  const items: ICommandItem[] = [
    ...pages.map((section) => ({ id: `page:${section.id}`, icon: section.icon, label: t(PAGE_COMMANDS[section.id]), kind: t('palette.kind.page') })),
    ...creates.map((command) => ({ id: `create:${command.id}`, icon: command.icon, label: t(command.label), kind: t('palette.kind.action') })),
    { id: 'sign-out', icon: 'out', label: t('palette.command.signOut'), kind: t('palette.kind.action') },
    {
      id: 'language',
      label: t('palette.command.language', { language: t(`language.${target}`) }),
      kind: t('palette.kind.action'),
    },
  ];

  const run = (item: ICommandItem) => {
    const page = pages.find((section) => `page:${section.id}` === item.id);
    const create = creates.find((command) => `create:${command.id}` === item.id);
    if (page) void navigate({ to: page.path });
    else if (create) void navigate({ to: create.path, search: { slide: create.slide } });
    else if (item.id === 'sign-out') void SignOut();
    else if (item.id === 'language') SetLocale(target);
  };

  return { items, run };
};

import { lazyPage } from '@/App/Navigation/LazyPage';
import { instanceOwner, member } from '@/App/Navigation/Gates';
import {
  canApproveDeployments,
  canManageMembers,
  canManageRunners,
  canManageTokens,
  canReadAudit,
  canReadMembers,
  canReadRunners,
  canReadSecrets,
} from '@/Core/Auth/AuthTypes';

export const SHELL_PAGES = {
  home: lazyPage(() => import('@/App/Pages/HomePage').then((module) => module.HomePage)),
  projects: lazyPage(() => import('@/App/Pages/ProjectsPage').then((module) => module.ProjectsPage), member),
  project: lazyPage(() => import('@/App/Pages/ProjectPage').then((module) => module.ProjectPage), member),
  projectSettings: lazyPage(
    () => import('@/App/Pages/ProjectSettingsPage').then((module) => module.ProjectSettingsPage),
    member,
  ),
  projectDeploy: lazyPage(
    () => import('@/App/Pages/ProjectDeployPage').then((module) => module.ProjectDeployPage),
    member,
  ),
  projectEnvironments: lazyPage(
    () => import('@/App/Pages/ProjectEnvironmentsPage').then((module) => module.ProjectEnvironmentsPage),
    member,
  ),
  projectIngest: lazyPage(
    () => import('@/App/Pages/ProjectIngestPage').then((module) => module.ProjectIngestPage),
    member,
  ),
  environment: lazyPage(() => import('@/App/Pages/EnvironmentPage').then((module) => module.EnvironmentPage), member),
  target: lazyPage(() => import('@/App/Pages/TargetPage').then((module) => module.TargetPage), member),
  run: lazyPage(() => import('@/App/Pages/RunPage').then((module) => module.RunPage), member),
  jobs: lazyPage(() => import('@/App/Pages/JobsPage').then((module) => module.JobsPage), member),
  approvals: lazyPage(
    () => import('@/App/Pages/ApprovalsPage').then((module) => module.ApprovalsPage),
    canApproveDeployments,
  ),
  job: lazyPage(() => import('@/App/Pages/JobPage').then((module) => module.JobPage), member),
  runners: lazyPage(() => import('@/App/Pages/RunnersPage').then((module) => module.RunnersPage), canReadRunners),
  runnerTokens: lazyPage(
    () => import('@/App/Pages/RunnerTokensPage').then((module) => module.RunnerTokensPage),
    canManageRunners,
  ),
  runner: lazyPage(() => import('@/App/Pages/RunnerPage').then((module) => module.RunnerPage), canReadRunners),
  members: lazyPage(() => import('@/App/Pages/MembersPage').then((module) => module.MembersPage), canReadMembers),
  member: lazyPage(() => import('@/App/Pages/MembersPage').then((module) => module.MembersPage), canReadMembers),
  teams: lazyPage(() => import('@/App/Pages/TeamsPage').then((module) => module.TeamsPage), canReadMembers),
  team: lazyPage(() => import('@/App/Pages/TeamsPage').then((module) => module.TeamsPage), canReadMembers),
  invitations: lazyPage(
    () => import('@/App/Pages/InvitationsPage').then((module) => module.InvitationsPage),
    canManageMembers,
  ),
  apiKeys: lazyPage(() => import('@/App/Pages/ApiKeysPage').then((module) => module.ApiKeysPage), canManageTokens),
  orgSecrets: lazyPage(() => import('@/App/Pages/OrgSecretsPage').then((module) => module.OrgSecretsPage), canReadSecrets),
  projectSecrets: lazyPage(
    () => import('@/App/Pages/ProjectSecretsPage').then((module) => module.ProjectSecretsPage),
    member,
  ),
  orgSettings: lazyPage(
    () => import('@/App/Pages/OrgSettingsPage').then((module) => module.OrgSettingsPage),
    member,
  ),
  audit: lazyPage(() => import('@/App/Pages/AuditPage').then((module) => module.AuditPage), canReadAudit),
  auditEvent: lazyPage(() => import('@/App/Pages/AuditEventPage').then((module) => module.AuditEventPage), canReadAudit),
  system: lazyPage(() => import('@/App/Pages/SystemPage').then((module) => module.SystemPage)),
  systemSettings: lazyPage(
    () => import('@/App/Pages/SystemSettingsPage').then((module) => module.SystemSettingsPage),
    instanceOwner,
  ),
  about: lazyPage(() => import('@/App/Pages/AboutPage').then((module) => module.AboutPage)),
  help: lazyPage(() => import('@/App/Pages/HelpPage').then((module) => module.HelpPage)),
  check: lazyPage(() => import('@/App/Pages/CheckPage').then((module) => module.CheckPage)),
  profile: lazyPage(() => import('@/App/Pages/ProfilePage').then((module) => module.ProfilePage)),
  security: lazyPage(() => import('@/App/Pages/SecurityPage').then((module) => module.SecurityPage)),
  preferences: lazyPage(() => import('@/App/Pages/PreferencesPage').then((module) => module.PreferencesPage)),
  organizations: lazyPage(() => import('@/App/Pages/OrganizationsPage').then((module) => module.OrganizationsPage)),
  notifications: lazyPage(() => import('@/App/Pages/NotificationsPage').then((module) => module.NotificationsPage)),
};

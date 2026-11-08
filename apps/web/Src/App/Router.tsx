import {
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
  type RouteComponent,
  type RouterHistory,
} from '@tanstack/react-router';

import { preloadShell } from '@/App/Layout/LazyShell';
import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { RootLayout } from '@/App/Layout/RootLayout';
import { ShellGate } from '@/App/Layout/ShellGate';
import { legacyStackHref } from '@/App/LegacyLinks';
import { parseJobsSearch } from '@/App/Navigation/JobsSearch';
import { SHELL_PAGES } from '@/App/Navigation/ShellPages';
import { parseShellSearch } from '@/App/Panels';
import { LoginRoute, preloadLoginPage } from '@/App/Routes/LoginRoute';
import { InviteRoute, preloadInvitePage } from '@/App/Routes/InviteRoute';
import { OidcCallbackRoute, preloadOidcCallbackPage } from '@/App/Routes/OidcCallbackRoute';
import { preloadSetupPage, SetupRoute } from '@/App/Routes/SetupRoute';
import { INVITE_PATH, takeInviteTokenFromHash } from '@/Modules/Identity/Store/InviteToken';
import { SETUP_PATH, takeSetupTokenFromHash } from '@/Modules/Identity/Store/SetupToken';
import { OIDC_CALLBACK_PATH, takeOidcCallbackFromSearch } from '@/Modules/Session/Store/OidcCallback';

const rootRoute = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFoundPage,
});

const shellRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'shell',
  component: ShellGate,
  validateSearch: parseShellSearch,
  beforeLoad: ({ search }) => {
    if (search.stack !== undefined) throw redirect({ href: legacyStackHref(search.stack), replace: true });
  },
  loaderDeps: ({ search }) => ({ slide: search.slide }),
  loader: ({ deps }) => {
    preloadShell(deps);
  },
});

const page = <TPath extends string>(path: TPath, component: RouteComponent) =>
  createRoute({ getParentRoute: () => shellRoute, path, component });

const homeRoute = page('/', SHELL_PAGES.home);
const projectsRoute = page('projects', SHELL_PAGES.projects);
const projectRoute = page('projects/$projectId', SHELL_PAGES.project);
const projectSettingsRoute = page('projects/$projectId/settings', SHELL_PAGES.projectSettings);
const projectSecretsRoute = page('projects/$projectId/secrets', SHELL_PAGES.projectSecrets);
const projectDeployRoute = page('projects/$projectId/deploy', SHELL_PAGES.projectDeploy);
const projectEnvironmentsRoute = page('projects/$projectId/environments', SHELL_PAGES.projectEnvironments);
const projectIngestRoute = page('projects/$projectId/ingest', SHELL_PAGES.projectIngest);
const environmentRoute = page('projects/$projectId/environments/$environmentName', SHELL_PAGES.environment);
const targetRoute = page('projects/$projectId/environments/$environmentName/targets/$targetName', SHELL_PAGES.target);
const newTargetRoute = page('projects/$projectId/environments/$environmentName/new-target', SHELL_PAGES.target);
const runRoute = page('projects/$projectId/runs/$runId', SHELL_PAGES.run);
const jobsRoute = createRoute({
  getParentRoute: () => shellRoute,
  path: 'jobs',
  component: SHELL_PAGES.jobs,
  validateSearch: parseJobsSearch,
});
const jobRoute = page('jobs/$jobId', SHELL_PAGES.job);
const approvalsRoute = page('approvals', SHELL_PAGES.approvals);
const runnersRoute = page('runners', SHELL_PAGES.runners);
const runnerTokensRoute = page('runners/tokens', SHELL_PAGES.runnerTokens);
const runnerRoute = createRoute({ getParentRoute: () => runnersRoute, path: '$runnerId', component: SHELL_PAGES.runner });
const membersRoute = page('org/members', SHELL_PAGES.members);
const memberRoute = page('org/members/$memberId', SHELL_PAGES.member);
const teamsRoute = page('org/teams', SHELL_PAGES.teams);
const teamRoute = page('org/teams/$teamId', SHELL_PAGES.team);
const invitationsRoute = page('org/invitations', SHELL_PAGES.invitations);
const apiKeysRoute = page('org/api-keys', SHELL_PAGES.apiKeys);
const orgSecretsRoute = page('org/secrets', SHELL_PAGES.orgSecrets);
const orgSettingsRoute = page('org/settings', SHELL_PAGES.orgSettings);
const auditRoute = page('audit', SHELL_PAGES.audit);
const auditEventRoute = page('audit/$eventId', SHELL_PAGES.auditEvent);
const systemRoute = page('system', SHELL_PAGES.system);
const checkRoute = page('system/checks/$check', SHELL_PAGES.check);
const systemSettingsRoute = page('system/settings', SHELL_PAGES.systemSettings);
const aboutRoute = page('about', SHELL_PAGES.about);
const helpRoute = page('help', SHELL_PAGES.help);
const profileRoute = page('account/profile', SHELL_PAGES.profile);
const securityRoute = page('account/security', SHELL_PAGES.security);
const preferencesRoute = page('account/preferences', SHELL_PAGES.preferences);
const organizationsRoute = page('organizations', SHELL_PAGES.organizations);
const notificationsRoute = page('notifications', SHELL_PAGES.notifications);
const shellNotFoundRoute = page('$', NotFoundPage);

const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: SETUP_PATH,
  beforeLoad: ({ location }) => {
    if (takeSetupTokenFromHash(location.hash)) throw redirect({ to: SETUP_PATH, hash: '', replace: true });
  },
  loader: preloadSetupPage,
  component: SetupRoute,
});

const inviteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: INVITE_PATH,
  beforeLoad: ({ location }) => {
    if (takeInviteTokenFromHash(location.hash)) throw redirect({ to: INVITE_PATH, hash: '', replace: true });
  },
  loader: preloadInvitePage,
  component: InviteRoute,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (search: Record<string, unknown>): { redirect?: string } =>
    typeof search.redirect === 'string' ? { redirect: search.redirect } : {},
  loader: preloadLoginPage,
  component: LoginRoute,
});

const oidcCallbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: OIDC_CALLBACK_PATH,
  beforeLoad: ({ location, params }) => {
    if (takeOidcCallbackFromSearch(location.pathname, location.searchStr)) {
      throw redirect({ to: OIDC_CALLBACK_PATH, params: { provider: params.provider }, search: {}, replace: true });
    }
  },
  loader: preloadOidcCallbackPage,
  component: OidcCallbackRoute,
});

const routeTree = rootRoute.addChildren([
  shellRoute.addChildren([
    homeRoute,
    projectsRoute,
    projectRoute,
    projectSettingsRoute,
    projectSecretsRoute,
    projectDeployRoute,
    projectEnvironmentsRoute,
    projectIngestRoute,
    environmentRoute,
    targetRoute,
    newTargetRoute,
    runRoute,
    jobsRoute,
    jobRoute,
    approvalsRoute,
    runnersRoute.addChildren([runnerRoute]),
    runnerTokensRoute,
    membersRoute,
    memberRoute,
    teamsRoute,
    teamRoute,
    invitationsRoute,
    apiKeysRoute,
    orgSecretsRoute,
    orgSettingsRoute,
    auditRoute,
    auditEventRoute,
    systemRoute,
    checkRoute,
    systemSettingsRoute,
    aboutRoute,
    helpRoute,
    profileRoute,
    securityRoute,
    preferencesRoute,
    organizationsRoute,
    notificationsRoute,
    shellNotFoundRoute,
  ]),
  setupRoute,
  inviteRoute,
  loginRoute,
  oidcCallbackRoute,
]);

export const createAppRouter = (history?: RouterHistory) => createRouter({ routeTree, history });

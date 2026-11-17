import type { TSectionId, TSectionPath } from '../Src/App/Navigation/Sections';
import type { TStaticSlideId } from '../Src/App/Panels';
import { LOG_JOB_ID } from './LogFixture';

export interface IPlaceContext {
  ProjectId: string;
  Environment: string;
  RunId: string;
}

export type TStub = 'log' | 'board';

export interface IPlace {
  Name: string;
  Path: string;
  Slide?: boolean;
  Stub?: TStub;
  Core?: boolean;
}

type TPathOf = (context: IPlaceContext) => string;

const at = (path: TSectionPath): TSectionPath => path;

const RUNNER_ENROLL = '/runners/tokens';
const ORG_ACCESS = '/org/api-keys';

export const SECTION_PLACES: Record<TSectionId, TSectionPath> = {
  home: at('/'),
  projects: at('/projects'),
  jobs: at('/jobs'),
  approvals: at('/approvals'),
  runners: at('/runners'),
  runnerTokens: at(RUNNER_ENROLL),
  members: at('/org/members'),
  teams: at('/org/teams'),
  invitations: at('/org/invitations'),
  apiKeys: at(ORG_ACCESS),
  secrets: at('/org/secrets'),
  orgSettings: at('/org/settings'),
  audit: at('/audit'),
  system: at('/system'),
  systemSettings: at('/system/settings'),
  about: at('/about'),
  help: at('/help'),
  profile: at('/account/profile'),
  security: at('/account/security'),
  preferences: at('/account/preferences'),
  organizations: at('/organizations'),
  notifications: at('/notifications'),
};

const project = (context: IPlaceContext): string => `/projects/${context.ProjectId}`;
const environment = (context: IPlaceContext): string => `${project(context)}/environments/${context.Environment}`;

export const SLIDE_PAGES: Record<TStaticSlideId, readonly TPathOf[]> = {
  'runner-new': [() => '/runners'],
  'organization-invite': [() => '/org/members'],
  'organization-transfer': [() => '/org/settings'],
  'organization-token-new': [() => ORG_ACCESS],
  'organization-new': [() => '/organizations'],
  'team-new': [() => '/org/teams'],
  'account-password': [() => '/account/security'],
  'account-totp': [() => '/account/security'],
  'account-token-new': [() => '/account/security'],
  'theme-edit': [() => '/account/preferences'],
  'member-remove': [],
  'organization-leave': [() => '/org/settings'],
  'oidc-new': [() => '/system/settings'],
  'project-new': [() => '/projects'],
  'repository-link': [project],
  'run-trigger': [project],
  'secret-new': [() => '/org/secrets', environment],
  'variable-new': [() => '/org/secrets', environment],
  'registry-new': [(context) => `${project(context)}/secrets`],
  'environment-new': [(context) => `${project(context)}/environments`],
  'ingest-token-new': [(context) => `${project(context)}/ingest`],
  'notification-channel-new': [(context) => `${project(context)}/notifications`],
};

export const SLIDE_EXEMPT: Record<string, string> = {
  'member-remove': 'needs a second member in the organization',
  'secret-edit-NAME': 'needs an existing secret; shares SecretFormBody with secret-new',
  'variable-edit-NAME': 'needs an existing variable; shares VariableFormBody with variable-new',
  'registry-edit-ID': 'needs an existing registry credential; shares RegistryFormBody with registry-new',
  'oidc-edit-ID': 'needs an existing provider; shares OidcProviderBody with oidc-new',
  'theme-edit-ID': 'needs a custom theme; shares ThemeEditorBody with theme-edit',
  'deployment-DECISION-ID': 'needs a pending approval; covered by the manual checklist',
};

const withSlide = (path: string, slide: string): string => `${path}?slide=${slide}`;

const corePlaces = (context: IPlaceContext): IPlace[] => [
  { Name: 'home', Path: SECTION_PLACES.home, Core: true },
  { Name: 'projects', Path: SECTION_PLACES.projects, Core: true },
  { Name: 'project', Path: project(context), Core: true },
  { Name: 'run', Path: `${project(context)}/runs/${context.RunId}`, Core: true },
  { Name: 'environment', Path: environment(context), Core: true },
];

const sectionPlaces = (): IPlace[] =>
  (Object.entries(SECTION_PLACES) as [TSectionId, TSectionPath][])
    .filter(([, path]) => path !== SECTION_PLACES.home && path !== SECTION_PLACES.projects)
    .map(([id, path]) => ({ Name: `section ${id}`, Path: path }));

const extraPlaces = (context: IPlaceContext): IPlace[] => [
  { Name: 'project settings', Path: `${project(context)}/settings` },
  { Name: 'project secrets', Path: `${project(context)}/secrets` },
  { Name: 'project environments', Path: `${project(context)}/environments` },
  { Name: 'project ingest', Path: `${project(context)}/ingest` },
  { Name: 'project notification channels', Path: `${project(context)}/notifications` },
  { Name: 'deploy board', Path: `${project(context)}/deploy`, Stub: 'board' },
  { Name: 'log viewer', Path: `/jobs/${LOG_JOB_ID}`, Stub: 'log' },
];

export const pagePlaces = (context: IPlaceContext): IPlace[] => [...corePlaces(context), ...sectionPlaces(), ...extraPlaces(context)];

export const slidePlaces = (context: IPlaceContext): IPlace[] =>
  (Object.entries(SLIDE_PAGES) as [TStaticSlideId, readonly TPathOf[]][]).flatMap(([slide, builders]) =>
    builders.map((build, index) => ({ Name: `slide ${slide} ${String(index + 1)}`, Path: withSlide(build(context), slide), Slide: true })),
  );

export const allPlaces = (context: IPlaceContext): IPlace[] => [...pagePlaces(context), ...slidePlaces(context)];

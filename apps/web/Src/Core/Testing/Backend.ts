import { LATEST_RELEASE } from '@/Core/Help/Generated/ReleaseMeta';
import { mfaReply, type IMfaFixture } from '@/Core/Testing/MfaBackend';
import { envelope, META_BODY, meBody, ORG_ID, READY_BODY, SESSION_ID, stubFetch, UNAUTHENTICATED_BODY, USER_ID } from '@/Core/Testing/Fetch';

export interface ISessionFixture {
  Id: string;
  CreatedAt: string;
  LastSeenAt: string;
  ExpiresAt: string;
  UserAgent: string;
  IpAddress: string;
  Current: boolean;
}

export const CHROME_UA = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36';
export const FIREFOX_UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0';
export const SAFARI_UA =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1';

export const sessionsFixture = (): ISessionFixture[] => [
  {
    Id: SESSION_ID,
    CreatedAt: '2026-10-27T08:00:00Z',
    LastSeenAt: new Date().toISOString(),
    ExpiresAt: '2026-11-10T08:00:00Z',
    UserAgent: CHROME_UA,
    IpAddress: '203.0.113.7',
    Current: true,
  },
  {
    Id: '0198a000-0000-7000-8000-0000000000c2',
    CreatedAt: '2026-10-26T08:00:00Z',
    LastSeenAt: new Date(Date.now() - 2 * 3_600_000).toISOString(),
    ExpiresAt: '2026-11-09T08:00:00Z',
    UserAgent: FIREFOX_UA,
    IpAddress: '198.51.100.20',
    Current: false,
  },
  {
    Id: '0198a000-0000-7000-8000-0000000000c3',
    CreatedAt: '2026-10-25T08:00:00Z',
    LastSeenAt: new Date(Date.now() - 26 * 3_600_000).toISOString(),
    ExpiresAt: '2026-11-08T08:00:00Z',
    UserAgent: SAFARI_UA,
    IpAddress: '2001:db8::1',
    Current: false,
  },
];

export interface IAuditFixture {
  Id: string;
  CreatedAt: string;
  OrganizationId: string;
  ActorUserId?: string;
  Action: string;
  TargetType: string;
  TargetId: string;
  Ip: string;
  Data: Record<string, string>;
}

export interface IMemberFixture {
  UserId: string;
  Name: string;
  Email: string;
  Role: string;
  EffectiveRole: string;
  JoinedAt: string;
  LastActiveAt?: string;
  Mfa?: string;
  MfaResetPending?: boolean;
  Teams: { Id: string; Name: string }[];
}

export const auditFixture = (count: number): IAuditFixture[] =>
  Array.from({ length: count }, (_, index) => ({
    Id: `0198a000-0000-7000-8000-${String(index + 1).padStart(12, '0')}`,
    CreatedAt: new Date(Date.now() - (index + 1) * 600_000).toISOString(),
    OrganizationId: ORG_ID,
    ActorUserId: index % 3 === 2 ? undefined : USER_ID,
    Action: index % 3 === 2 ? 'auth.login_failed' : index % 3 === 1 ? 'auth.password_changed' : 'auth.login',
    TargetType: 'session',
    TargetId: SESSION_ID,
    Ip: `203.0.113.${index + 1}`,
    Data: {},
  }));

export const membersFixture = (): IMemberFixture[] => [
  { UserId: USER_ID, Name: 'Sabry Dawood', Email: 'sabry@future.dev', Role: 'owner', EffectiveRole: 'owner', JoinedAt: '2026-10-01T08:00:00Z', Mfa: 'passkey', Teams: [] },
  {
    UserId: '0198a000-0000-7000-8000-0000000000b2', Name: 'Mona Adel', Email: 'mona@future.dev', Role: 'admin', EffectiveRole: 'admin',
    JoinedAt: '2026-10-05T08:00:00Z', Mfa: 'totp', Teams: [],
  },
  {
    UserId: '0198a000-0000-7000-8000-0000000000b3', Name: 'Hana Samir', Email: 'hana@future.dev', Role: 'developer', EffectiveRole: 'developer',
    JoinedAt: '2026-10-10T08:00:00Z', LastActiveAt: '2026-10-28T09:30:00Z', Mfa: 'none', Teams: [{ Id: '0198a000-0000-7000-8000-0000000000d1', Name: 'Backend' }],
  },
];

export interface IOverviewFixture {
  Jobs: { Running: number; Queued: number; QueuedNoRunner: number; FailedLast24h: number } | null;
  Runners: { Online: number; Offline: number; Paused: number } | null;
  Invitations: { Pending: number } | null;
  Approvals: { Pending: number } | null;
}

const RUNNER_READERS = ['owner', 'admin', 'maintainer', 'developer'];
const MEMBER_MANAGERS = ['owner', 'admin'];

export const overviewFixture =
  (counts: { Jobs?: Partial<NonNullable<IOverviewFixture['Jobs']>>; Runners?: Partial<NonNullable<IOverviewFixture['Runners']>>; Pending?: number; Approvals?: number } = {}) =>
  (role: string): IOverviewFixture => ({
    Jobs: { Running: 0, Queued: 0, QueuedNoRunner: 0, FailedLast24h: 0, ...counts.Jobs },
    Runners: RUNNER_READERS.includes(role) ? { Online: 0, Offline: 0, Paused: 0, ...counts.Runners } : null,
    Invitations: MEMBER_MANAGERS.includes(role) ? { Pending: counts.Pending ?? 0 } : null,
    Approvals: RUNNER_READERS.includes(role) ? { Pending: counts.Approvals ?? 0 } : null,
  });

export type TRouteHandler = (request: Request, url: URL) => Response | Promise<Response> | undefined;

export interface IPreferencesFixture {
  Locale: string;
  ThemeId: string;
  CustomThemes: unknown[];
  OrganizationId: string | null;
  Saved: boolean;
  ReleaseNotesSeen?: string | null;
}

export const defaultPreferences = (): IPreferencesFixture => ({
  Locale: 'ar',
  ThemeId: 'sea',
  CustomThemes: [],
  OrganizationId: null,
  Saved: true,
  ReleaseNotesSeen: LATEST_RELEASE,
});

export interface IBackendOptions {
  role?: string;
  signedIn?: boolean;
  setupRequired?: boolean;
  sessions?: ISessionFixture[];
  routes?: TRouteHandler[];
  passwordReply?: () => Response;
  audit?: IAuditFixture[];
  members?: IMemberFixture[];
  secondFactorRule?: 'off' | 'admins' | 'everyone';
  verify?: () => Response;
  renameReply?: () => Response;
  mfa?: IMfaFixture;
  providers?: unknown[];
  identities?: unknown[];
  tokens?: unknown[];
  preferences?: IPreferencesFixture;
  overview?: (role: string) => IOverviewFixture;
  instanceOwner?: boolean;
}

export interface IBackendState {
  signedIn: boolean;
  sessions: ISessionFixture[];
  role: string;
  csrf: string;
  orgName: string;
  mfa: IMfaFixture;
  preferences: IPreferencesFixture;
}

const failure = (Code: string, Message: string, status: number, headers: Record<string, string> = {}): Response =>
  new Response(JSON.stringify({ Success: false, Error: { Code, Message } }), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  });

export const backendFailure = failure;

const SESSION_PATH = /\/sessions\/([0-9a-f-]{36})$/;

export const startBackend = (options: IBackendOptions = {}) => {
  const state: IBackendState = {
    signedIn: options.signedIn ?? true,
    sessions: options.sessions ?? sessionsFixture(),
    role: options.role ?? 'owner',
    csrf: 'csrf-token-1',
    orgName: 'Future Solution',
    mfa: options.mfa ?? { TotpEnabled: false, RecoveryCodesRemaining: 0, Passkeys: [] },
    preferences: options.preferences ?? defaultPreferences(),
  };
  const stub = stubFetch(async (request) => {
    const url = new URL(request.url);
    const { pathname } = url;
    for (const route of options.routes ?? []) {
      const handled = route(request, url);
      if (handled) return handled;
    }
    if (pathname.endsWith('/meta')) {
      return envelope({ ...META_BODY, Data: { ...META_BODY.Data, SetupRequired: options.setupRequired ?? false } });
    }
    if (pathname.endsWith('/health/ready')) return envelope(READY_BODY);
    if (pathname.endsWith('/me') && request.method === 'GET') {
      return state.signedIn ? envelope(meBody(state.role, state.csrf, state.orgName, options.instanceOwner ?? false)) : envelope(UNAUTHENTICATED_BODY, 401);
    }
    const access = await accessReply(request, pathname, options, state);
    if (access) return access;
    const mfa = await mfaReply(request, pathname, state.mfa);
    if (mfa) return mfa;
    const org = /\/organizations\/[0-9a-f-]{36}(\/[a-z-]+)*$/.exec(pathname);
    if (org) return organizationReply(request, url, options, state);
    if (pathname.endsWith('/me/password') && request.method === 'PUT') {
      if (options.passwordReply) return options.passwordReply();
      const revoked = state.sessions.filter((session) => !session.Current).length;
      state.sessions = state.sessions.filter((session) => session.Current);
      return envelope({ Success: true, Data: { Revoked: revoked } });
    }
    if (pathname.endsWith('/sessions') && request.method === 'GET') return envelope({ Success: true, Data: state.sessions });
    if (pathname.endsWith('/sessions/current') && request.method === 'DELETE') {
      state.signedIn = false;
      return envelope({ Success: true, Message: 'session.logout.succeeded' });
    }
    if (pathname.endsWith('/sessions/revoke-others') && request.method === 'POST') {
      const revoked = state.sessions.filter((session) => !session.Current).length;
      state.sessions = state.sessions.filter((session) => session.Current);
      return envelope({ Success: true, Data: { Revoked: revoked } });
    }
    const target = SESSION_PATH.exec(pathname)?.[1];
    if (target !== undefined && request.method === 'DELETE') {
      state.sessions = state.sessions.filter((session) => session.Id !== target);
      return envelope({ Success: true, Message: 'session.revoke.succeeded' });
    }
    return failure('NOT_FOUND', 'x', 404);
  });
  return { ...stub, state, orgId: ORG_ID };
};

const accessReply = async (request: Request, pathname: string, options: IBackendOptions, state: IBackendState): Promise<Response | null> => {
  if (pathname.endsWith('/oidc/providers')) return envelope({ Success: true, Data: options.providers ?? [] });
  if (pathname.endsWith('/me/identities') && request.method === 'GET') return envelope({ Success: true, Data: options.identities ?? [] });
  if (pathname.endsWith('/me/api-tokens') && request.method === 'GET') return envelope({ Success: true, Data: options.tokens ?? [] });
  if (pathname.endsWith('/me/release-notes-seen') && request.method === 'PUT') {
    const body = (await request.clone().json()) as { Version: string };
    state.preferences = { ...state.preferences, ReleaseNotesSeen: body.Version };
    return envelope({ Success: true, Data: state.preferences });
  }
  if (pathname.endsWith('/me/preferences')) {
    if (request.method === 'PUT') {
      const body = (await request.clone().json()) as Omit<IPreferencesFixture, 'Saved'>;
      state.preferences = { ...body, Saved: true, ReleaseNotesSeen: state.preferences.ReleaseNotesSeen };
    }
    return envelope({ Success: true, Data: state.preferences });
  }
  return null;
};

const pageOf = (items: readonly unknown[], url: URL): Response => {
  const page = Number(url.searchParams.get('Page') ?? '1');
  const limit = Number(url.searchParams.get('Limit') ?? '25');
  return envelope({
    Success: true,
    Data: items.slice((page - 1) * limit, page * limit),
    Meta: { Page: page, Limit: limit, Total: items.length },
  });
};

const organizationReply = async (request: Request, url: URL, options: IBackendOptions, state: IBackendState): Promise<Response> => {
  const { pathname } = url;
  const organization = (name: string) => ({
    Id: ORG_ID,
    Name: name,
    Slug: 'future-solution',
    CreatedAt: '2026-10-01T08:00:00Z',
    RequireSecondFactor: options.secondFactorRule ?? 'off',
  });
  if (pathname.endsWith('/audit-events/verify')) {
    return options.verify?.() ?? envelope({ Success: true, Data: { Valid: true, Checked: (options.audit ?? []).length } });
  }
  if (pathname.endsWith('/overview')) return envelope({ Success: true, Data: (options.overview ?? overviewFixture())(state.role) });
  if (pathname.endsWith('/audit-events')) return pageOf(options.audit ?? [], url);
  if (pathname.endsWith('/members')) return pageOf(options.members ?? membersFixture(), url);
  if (request.method === 'GET' && /\/(runners|jobs|runner-registration-tokens)$/.test(pathname)) return pageOf([], url);
  if (request.method === 'GET' && /\/(teams|invitations|api-tokens|secrets|variables|registry-credentials)$/.test(pathname)) return envelope({ Success: true, Data: [] });
  if (request.method === 'PATCH') {
    if (options.renameReply) return options.renameReply();
    const body = (await request.clone().json()) as { Name: string };
    state.orgName = body.Name;
    return envelope({ Success: true, Data: organization(state.orgName) });
  }
  return envelope({ Success: true, Data: organization(state.orgName) });
};

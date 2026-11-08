import type { TRouteHandler } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';

export const CHANNEL_ID = '0198a000-0000-7000-8000-000000000a01';
export const OTHER_CHANNEL_ID = '0198a000-0000-7000-8000-000000000a02';
export const NEW_CHANNEL_ID = '0198a000-0000-7000-8000-000000000a09';
export const DELIVERY_ID = '0198a000-0000-7000-8000-000000000b01';
export const NEW_SECRET = ['dhow', 'whsec', 'kP7mQ2vX9tN4cL8bR1wY6hJ3dF5gZ0aS'].join('_');

export interface INotificationWrite {
  Method: string;
  Path: string;
  Body: Record<string, unknown> | null;
}

export interface INotificationsState {
  channels: Record<string, unknown>[];
  rules: Record<string, unknown>[];
  deliveries: Record<string, unknown>[];
  inbox: Record<string, unknown>[];
  muted: boolean;
  writes: INotificationWrite[];
  rateLimited: boolean;
  failCreate: Response | null;
  failUnread: boolean;
  calls: string[];
}

export const channelFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: CHANNEL_ID,
  ProjectId: '0198a000-0000-7000-8000-000000000301',
  Kind: 'webhook',
  Name: 'ops-hook',
  Locale: 'en',
  Enabled: true,
  Host: 'hooks.example.com',
  HasSecret: true,
  Version: 1,
  ConsecutiveFailures: 0,
  CreatedByUserId: '0198a000-0000-7000-8000-0000000000b1',
  CreatedAt: '2026-11-01T08:00:00Z',
  UpdatedAt: '2026-11-01T08:00:00Z',
  ...over,
});

export const ruleFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  ChannelId: CHANNEL_ID,
  Events: ['deployment.finished'],
  Environments: [],
  Statuses: ['failed'],
  Roles: [],
  Version: 1,
  ...over,
});

export const deliveryFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: DELIVERY_ID,
  ChannelId: CHANNEL_ID,
  ChannelName: 'ops-hook',
  ChannelKind: 'webhook',
  Event: 'deployment.finished',
  EventId: '0198a000-0000-7000-8000-000000000c01',
  State: 'dead',
  Attempts: 5,
  NextAttemptAt: '2026-11-08T08:00:00Z',
  LastAttemptAt: '2026-11-08T08:00:00Z',
  LastError: 'http_status',
  LastStatus: 502,
  CreatedAt: '2026-11-08T07:00:00Z',
  ...over,
});

export const inboxFixture = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  Id: '0198a000-0000-7000-8000-000000000d01',
  EventId: '0198a000-0000-7000-8000-000000000c01',
  Event: 'deployment.finished',
  TitleKey: 'inbox.deployment.finished.failed',
  Params: { project: 'Web app', environment: 'production', run: '12' },
  Link: '/projects/0198a000-0000-7000-8000-000000000301/runs/12',
  Read: false,
  CreatedAt: '2026-11-08T07:00:00Z',
  ...over,
});

export const newNotificationsState = (): INotificationsState => ({
  channels: [channelFixture(), channelFixture({ Id: OTHER_CHANNEL_ID, Kind: 'email', Name: 'team-mail', Host: undefined, HasSecret: false })],
  rules: [ruleFixture(), ruleFixture({ ChannelId: OTHER_CHANNEL_ID, Roles: ['owner', 'admin'] })],
  deliveries: [deliveryFixture(), deliveryFixture({ Id: '0198a000-0000-7000-8000-000000000b02', State: 'sent', Attempts: 1, LastError: '' })],
  inbox: [inboxFixture(), inboxFixture({ Id: '0198a000-0000-7000-8000-000000000d02', Read: true, TitleKey: 'inbox.run.finished', Link: undefined })],
  muted: false,
  writes: [],
  rateLimited: false,
  failCreate: null,
  failUnread: false,
  calls: [],
});

const PROJECT_PATH =
  /^\/api\/v1\/organizations\/[^/]+\/projects\/[^/]+\/(notification-channels(?:\/([^/]+)(?:\/(remove|test))?)?|notification-rules|notification-deliveries(?:\/([^/]+)\/retry)?)$/;
const ME_PATH = /^\/api\/v1\/me\/(notification-mute|notifications(?:\/(unread-count|read-all|([^/]+)\/read))?)$/;

const pageOf = (items: readonly unknown[], url: URL): Response => {
  const page = Number(url.searchParams.get('Page') ?? '1');
  const limit = Number(url.searchParams.get('Limit') ?? '20');
  return envelope({ Success: true, Data: items.slice((page - 1) * limit, page * limit), Meta: { Page: page, Limit: limit, Total: items.length } });
};

const bodyOf = async (request: Request): Promise<Record<string, unknown> | null> => {
  try {
    return (await request.clone().json()) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const rateLimited = (): Response =>
  new Response(JSON.stringify({ Success: false, Error: { Code: 'TEST_RATE_LIMITED', Message: 'أرسلت رسائل اختبار كثيرة. انتظر دقيقة.' } }), {
    status: 429,
    headers: { 'Content-Type': 'application/json', 'Retry-After': '60' },
  });

const channelWrite = (state: INotificationsState, method: string, id: string | undefined, action: string | undefined, body: Record<string, unknown> | null) => {
  if (method === 'POST' && id === undefined) {
    const kind = typeof body?.Kind === 'string' ? body.Kind : 'webhook';
    const channel = channelFixture({ Id: NEW_CHANNEL_ID, Kind: kind, Name: body?.Name, Locale: body?.Locale ?? 'ar', HasSecret: kind !== 'email' && kind !== 'inapp' });
    state.channels = [...state.channels, channel];
    state.rules = [...state.rules, ruleFixture({ ChannelId: NEW_CHANNEL_ID })];
    return envelope({ Success: true, Data: kind === 'webhook' ? { Channel: channel, Secret: NEW_SECRET } : { Channel: channel } }, 201);
  }
  if (method === 'PUT') {
    state.channels = state.channels.map((channel) =>
      channel.Id === id ? { ...channel, Enabled: body?.Enable === true ? true : channel.Enabled, DisabledReason: undefined, Version: Number(channel.Version) + 1 } : channel,
    );
    const updated = state.channels.find((channel) => channel.Id === id);
    return envelope({ Success: true, Data: body?.RotateSecret === true ? { Channel: updated, Secret: NEW_SECRET } : { Channel: updated } }, 200);
  }
  if (action === 'remove') {
    const removed = state.channels.find((channel) => channel.Id === id);
    state.channels = state.channels.filter((channel) => channel.Id !== id);
    return envelope({ Success: true, Data: removed }, 200);
  }
  return envelope({ Success: true, Data: { EventId: '0198a000-0000-7000-8000-000000000c09' } }, 202);
};

const projectRoute = async (state: INotificationsState, request: Request, url: URL, match: RegExpExecArray): Promise<Response> => {
  const [, kind, id, action, deliveryId] = match;
  if (request.method === 'GET') {
    if (kind === 'notification-rules') return envelope({ Success: true, Data: state.rules });
    if (kind?.startsWith('notification-deliveries')) {
      const filter = url.searchParams.get('State');
      return pageOf(filter === null ? state.deliveries : state.deliveries.filter((delivery) => delivery.State === filter), url);
    }
    return pageOf(state.channels, url);
  }
  const body = await bodyOf(request);
  state.writes.push({ Method: request.method, Path: url.pathname.replace(/^\/api\/v1/, ''), Body: body });
  if (action === 'test' && state.rateLimited) return rateLimited();
  if (kind === 'notification-rules') {
    const incoming = (body?.Rules ?? []) as Record<string, unknown>[];
    state.rules = state.rules.map((rule) => incoming.find((next) => next.ChannelId === rule.ChannelId) ?? rule);
    return envelope({ Success: true, Data: incoming });
  }
  if (deliveryId !== undefined) {
    state.deliveries = state.deliveries.map((delivery) => (delivery.Id === deliveryId ? { ...delivery, State: 'pending' } : delivery));
    return envelope({ Success: true, Data: { Retried: true } });
  }
  if (state.failCreate && request.method === 'POST' && id === undefined) return state.failCreate.clone();
  return channelWrite(state, request.method, id, action, body);
};

const meRoute = async (state: INotificationsState, request: Request, url: URL, match: RegExpExecArray): Promise<Response> => {
  const [, kind, sub, readId] = match;
  if (kind === 'notification-mute') {
    if (request.method === 'PUT') state.muted = (await bodyOf(request))?.Muted === true;
    return envelope({ Success: true, Data: { Muted: state.muted } });
  }
  const unread = state.inbox.filter((item) => item.Read !== true);
  if (sub === 'unread-count') {
    if (state.failUnread) return envelope({ Success: false, Error: { Code: 'INTERNAL', Message: 'x' } }, 500);
    return envelope({ Success: true, Data: { Unread: unread.length } });
  }
  if (request.method === 'GET') {
    const only = url.searchParams.get('Unread') === 'true';
    return pageOf(only ? unread : state.inbox, url);
  }
  state.writes.push({ Method: request.method, Path: url.pathname.replace(/^\/api\/v1/, ''), Body: null });
  if (sub === 'read-all') {
    const marked = unread.length;
    state.inbox = state.inbox.map((item) => ({ ...item, Read: true }));
    return envelope({ Success: true, Data: { Marked: marked } });
  }
  state.inbox = state.inbox.map((item) => (item.Id === readId ? { ...item, Read: true } : item));
  return envelope({ Success: true, Data: state.inbox.find((item) => item.Id === readId) });
};

export const notificationsRoute =
  (state: INotificationsState): TRouteHandler =>
  (request, url) => {
    const project = PROJECT_PATH.exec(url.pathname);
    if (project) return projectRoute(state, request, url, project);
    const me = ME_PATH.exec(url.pathname);
    if (me) return meRoute(state, request, url, me);
    return undefined;
  };

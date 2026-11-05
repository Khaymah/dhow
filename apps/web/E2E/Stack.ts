import { execFile } from 'node:child_process';
import { createHmac } from 'node:crypto';
import { promisify } from 'node:util';

const run = promisify(execFile);
const BASE32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
const STEP_MS = 30_000;

export interface ISession {
  Cookie: string;
  Csrf: string;
}

export interface IReply {
  Status: number;
  Data: unknown;
  Cookies: string[];
}

export interface IUser {
  Email: string;
  Password: string;
  Name: string;
  TotpSecret?: string;
}

export const setting = (name: string): string => {
  const value = process.env[name];
  if (value === undefined || value === '') throw new Error(`${name} is not set: run the tests with deploy/e2e/run.sh`);
  return value;
};

export const stack = () => ({
  Url: setting('DHOW_E2E_URL'),
  OrgId: setting('DHOW_E2E_ORG_ID'),
  ProjectId: setting('DHOW_E2E_PROJECT_ID'),
  Owner: { Email: setting('DHOW_E2E_EMAIL'), Password: setting('DHOW_E2E_PASSWORD'), Name: 'Demo Owner' } satisfies IUser,
});

export const field = (value: unknown, ...path: string[]): unknown => {
  let current = value;
  for (const key of path) {
    if (typeof current !== 'object' || current === null || !(key in current)) return undefined;
    current = (current as Record<string, unknown>)[key];
  }
  return current;
};

export const text = (value: unknown, ...path: string[]): string => {
  const found = field(value, ...path);
  if (typeof found !== 'string') throw new Error(`no string at ${path.join('.')} in ${JSON.stringify(value)}`);
  return found;
};

export const list = (value: unknown, ...path: string[]): unknown[] => {
  const found = field(value, ...path);
  return Array.isArray(found) ? (found as unknown[]) : [];
};

const decode = (secret: string): Buffer => {
  let bits = '';
  for (const char of secret.replace(/=+$/, '').toUpperCase()) {
    const index = BASE32.indexOf(char);
    if (index < 0) throw new Error('the TOTP secret is not base32');
    bits += index.toString(2).padStart(5, '0');
  }
  const bytes: number[] = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) bytes.push(parseInt(bits.slice(i, i + 8), 2));
  return Buffer.from(bytes);
};

export const totpAt = (secret: string, step: number): string => {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(step));
  const digest = createHmac('sha1', decode(secret)).update(counter).digest();
  const offset = (digest.at(-1) ?? 0) & 0x0f;
  const value = digest.readUInt32BE(offset) & 0x7fffffff;
  return (value % 1_000_000).toString().padStart(6, '0');
};

const usedSteps = new Map<string, number>();

export const freshCode = async (secret: string): Promise<string> => {
  for (;;) {
    const step = Math.floor(Date.now() / STEP_MS);
    if ((usedSteps.get(secret) ?? -1) < step && Date.now() % STEP_MS < STEP_MS - 3_000) {
      usedSteps.set(secret, step);
      return totpAt(secret, step);
    }
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
};

export const call = async (method: string, path: string, body?: unknown, session?: ISession, cookie?: string): Promise<IReply> => {
  const { Url } = stack();
  const headers: Record<string, string> = { 'Content-Type': 'application/json', Origin: Url };
  if (session) {
    headers.Cookie = session.Cookie;
    headers['X-CSRF-Token'] = session.Csrf;
  } else if (cookie) {
    headers.Cookie = cookie;
  }
  const response = await fetch(`${Url}/api/v1${path}`, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const raw = await response.text();
  const parsed: unknown = raw === '' ? null : JSON.parse(raw);
  const cookies = response.headers.getSetCookie().map((value) => value.split(';')[0] ?? '');
  if (response.status >= 400) throw new Error(`${method} ${path} answered ${response.status}: ${JSON.stringify(field(parsed, 'Error'))}`);
  return { Status: response.status, Data: field(parsed, 'Data'), Cookies: cookies };
};

const cookieNamed = (cookies: string[], name: string): string => {
  const found = cookies.find((value) => value.split('=')[0]?.endsWith(name));
  if (found === undefined) throw new Error(`no ${name} cookie in ${cookies.map((value) => value.split('=')[0]).join(', ')}`);
  return found;
};

export const signIn = async (user: IUser): Promise<ISession> => {
  const first = await call('POST', '/sessions', { Email: user.Email, Password: user.Password });
  if (first.Status === 202) {
    if (user.TotpSecret === undefined) throw new Error(`${user.Email} needs a second step`);
    const mfa = cookieNamed(first.Cookies, 'dhow_mfa');
    const second = await call('POST', '/sessions/mfa', { Method: 'totp', Code: await freshCode(user.TotpSecret) }, undefined, mfa);
    return { Cookie: cookieNamed(second.Cookies, 'dhow_session'), Csrf: text(second.Data, 'CsrfToken') };
  }
  return { Cookie: cookieNamed(first.Cookies, 'dhow_session'), Csrf: text(first.Data, 'CsrfToken') };
};

export const invite = async (owner: ISession, user: IUser, role: string): Promise<void> => {
  const { OrgId } = stack();
  const issued = await call('POST', `/organizations/${OrgId}/invitations`, { Email: user.Email, Role: role }, owner);
  const token = text(issued.Data, 'Link').split('#')[1] ?? '';
  await call('POST', '/invitations/accept', { Token: token, Name: user.Name, Password: user.Password });
};

export const turnOnTotp = async (user: IUser): Promise<{ User: IUser; Session: ISession }> => {
  const session = await signIn(user);
  const factor = { Method: 'password', Password: user.Password };
  const enrollment = await call('POST', '/me/totp', { CurrentFactor: factor }, session);
  const secret = text(enrollment.Data, 'Secret');
  await call('POST', '/me/totp/confirm', { Code: await freshCode(secret), CurrentFactor: factor }, session);
  return { User: { ...user, TotpSecret: secret }, Session: session };
};

export const setLanguage = async (session: ISession, lang: string): Promise<void> => {
  const current = (await call('GET', '/me/preferences', undefined, session)).Data;
  await call(
    'PUT',
    '/me/preferences',
    { Locale: lang, ThemeId: field(current, 'ThemeId'), CustomThemes: field(current, 'CustomThemes') ?? [], OrganizationId: field(current, 'OrganizationId') ?? null },
    session,
  );
};

export const push = async (branch: string): Promise<void> => {
  await run(setting('DHOW_E2E_PUSH'), [branch], { env: process.env, timeout: 120_000 });
};

export const runOn = async (session: ISession, ref: string, after: number): Promise<{ Id: string; Number: number }> => {
  const { OrgId, ProjectId } = stack();
  for (let attempt = 0; attempt < 300; attempt += 1) {
    const runs = await call('GET', `/organizations/${OrgId}/projects/${ProjectId}/runs`, undefined, session);
    const found = list(runs.Data)
      .map((entry) => ({ Id: field(entry, 'Id'), Number: field(entry, 'Number'), Ref: field(entry, 'Ref') }))
      .find((entry) => typeof entry.Number === 'number' && entry.Number > after && (entry.Ref === ref || entry.Ref === `refs/heads/${ref}`));
    if (found && typeof found.Id === 'string' && typeof found.Number === 'number') return { Id: found.Id, Number: found.Number };
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  throw new Error(`no run on ${ref} after run ${after}`);
};

export const waitRun = async (session: ISession, id: string, states: string[]): Promise<string> => {
  const { OrgId, ProjectId } = stack();
  let last = '';
  for (let attempt = 0; attempt < 600; attempt += 1) {
    const reply = await call('GET', `/organizations/${OrgId}/projects/${ProjectId}/runs/${id}`, undefined, session);
    const state = field(reply.Data, 'Run', 'State') ?? field(reply.Data, 'State');
    last = typeof state === 'string' ? state : '';
    if (states.includes(last)) return last;
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  throw new Error(`run ${id} is ${last}, not one of ${states.join(', ')}`);
};

export const waitGate = async (session: ISession, id: string, gate: string): Promise<void> => {
  const { OrgId, ProjectId } = stack();
  for (let attempt = 0; attempt < 600; attempt += 1) {
    const reply = await call('GET', `/organizations/${OrgId}/projects/${ProjectId}/runs/${id}`, undefined, session);
    if (list(reply.Data, 'Jobs').some((job) => field(job, 'Gate') === gate)) return;
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  throw new Error(`no job of run ${id} reached the ${gate} gate`);
};

import { backendFailure, type TRouteHandler } from '@/Core/Testing/Backend';
import { envelope } from '@/Core/Testing/Fetch';
import { HOST_ONE, HOST_TWO, targetFixture, type ITargetWrite } from '@/Core/Testing/TargetsBackend';

export const FINGERPRINT = 'SHA256:AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfG';
export const OTHER_FINGERPRINT = 'SHA256:ZyXwVuTsRqPoNmLkJiHgFeDcBa9876543210ZyXwVuT';
export const PUBLIC_KEY = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPublicKeyOfTheTarget deploy@dhow';
export const KEY_MATERIAL = 'fake-key-material-first-line\nfake-key-material-second-line';

export interface ISecretWrite {
  Name: string;
  Value: string;
}

export interface ITargetsManageState {
  target: Record<string, unknown>;
  fingerprintOnFetch: string;
  fetchFailure: Response | null;
  writes: ITargetWrite[];
  secrets: ISecretWrite[];
}

export const newTargetsManageState = (over: Record<string, unknown> = {}): ITargetsManageState => ({
  target: targetFixture({
    Hosts: [
      { Id: HOST_ONE, Position: 1, Host: 'web-1.example.com', Port: 22, KeyPinned: true, HostKeyFingerprint: FINGERPRINT },
      { Id: HOST_TWO, Position: 2, Host: 'web-2.example.com', Port: 22, KeyPinned: false },
    ],
    Ready: false,
    NotReady: ['host_key_unconfirmed', 'key_missing'],
    ...over,
  }),
  fingerprintOnFetch: FINGERPRINT,
  fetchFailure: null,
  writes: [],
  secrets: [],
});

const PATH =
  /^\/api\/v1\/organizations\/[^/]+\/projects\/[^/]+\/environments\/[^/]+\/(?:targets(?:\/([^/]+)(?:\/(remove|generate-key|hosts|hosts-order)|\/hosts\/([^/]+)(?:\/(remove|fetch-host-key|pending-host-key|confirm-host-key))?)?)?|secrets\/([^/]+))$/;

const ok = (data: unknown, status = 200): Response => envelope({ Success: true, Data: data }, status);

const bodyOf = async (request: Request): Promise<Record<string, unknown>> => {
  try {
    return (await request.clone().json()) as Record<string, unknown>;
  } catch {
    return {};
  }
};

type THost = Record<string, unknown>;

const hostsOf = (state: ITargetsManageState): THost[] => state.target.Hosts as THost[];

const mutateHost = (state: ITargetsManageState, hostId: string | undefined, patch: THost): void => {
  state.target = { ...state.target, Hosts: hostsOf(state).map((host) => (host.Id === hostId ? { ...host, ...patch } : host)) };
};

const bump = (state: ITargetsManageState, patch: THost = {}): void => {
  state.target = { ...state.target, ...patch, Version: (state.target.Version as number) + 1 };
};

const hostAction = (state: ITargetsManageState, hostId: string | undefined, action: string | undefined, body: THost): Response => {
  if (action === 'fetch-host-key') {
    if (state.fetchFailure) return state.fetchFailure.clone();
    mutateHost(state, hostId, { PendingHostKeyFingerprint: state.fingerprintOnFetch });
  } else if (action === 'pending-host-key') {
    mutateHost(state, hostId, { PendingHostKeyFingerprint: state.fingerprintOnFetch });
  } else if (action === 'confirm-host-key') {
    const host = hostsOf(state).find((item) => item.Id === hostId);
    if (host?.PendingHostKeyFingerprint !== body.Fingerprint) return backendFailure('HOST_KEY_FINGERPRINT_MISMATCH', 'البصمة لا تطابق.', 409);
    mutateHost(state, hostId, { KeyPinned: true, HostKeyFingerprint: body.Fingerprint, PendingHostKeyFingerprint: undefined });
  } else if (action === 'remove') {
    bump(state, { Hosts: hostsOf(state).filter((host) => host.Id !== hostId) });
  } else {
    bump(state);
    mutateHost(state, hostId, { Host: body.Host, Port: body.Port ?? 22, KeyPinned: false, HostKeyFingerprint: undefined });
  }
  return ok(state.target);
};

const targetAction = (state: ITargetsManageState, action: string | undefined, body: THost): Response => {
  if (action === 'generate-key') bump(state, { PublicKey: PUBLIC_KEY, KeySecretName: 'WEB_SSH_KEY', NotReady: [] });
  else if (action === 'hosts') {
    const added = { Id: `0198a000-0000-7000-8000-0000000007a${String(hostsOf(state).length + 4)}`, Position: hostsOf(state).length + 1, Host: body.Host, Port: body.Port ?? 22, KeyPinned: false };
    bump(state, { Hosts: [...hostsOf(state), added] });
  } else if (action === 'hosts-order') {
    const ids = body.HostIds as string[];
    bump(state, { Hosts: ids.map((id, index) => ({ ...hostsOf(state).find((host) => host.Id === id), Position: index + 1 })) });
  } else bump(state, { ...body, CurrentFactor: undefined, Version: undefined });
  return ok(state.target);
};

export const targetsManageRoute =
  (state: ITargetsManageState): TRouteHandler =>
  (request, url) => {
    const match = PATH.exec(url.pathname);
    if (!match) return undefined;
    const [, name, action, hostId, hostAction_, secretName] = match;
    if (request.method === 'GET') return name === undefined ? undefined : ok(state.target);
    return bodyOf(request).then((body) => {
      state.writes.push({ Method: request.method, Path: url.pathname.replace(/^\/api\/v1/, ''), Body: body });
      if (secretName !== undefined) {
        state.secrets.push({ Name: secretName, Value: String(body.Value) });
        return ok({ Name: secretName, Scope: 'environment' });
      }
      if (name === undefined) {
        state.target = { ...state.target, Name: body.Name, Kind: body.Kind, Version: 1 };
        return ok(state.target, 201);
      }
      if (hostId !== undefined) return hostAction(state, hostId, hostAction_, body);
      if (action === 'remove') return ok(state.target);
      return targetAction(state, action, body);
    });
  };

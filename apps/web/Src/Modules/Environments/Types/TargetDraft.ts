import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import type { TCreateTargetInput, TTargetSettings, TUpdateTargetInput } from '@/Modules/Environments/Api/TargetWritesApi';
import { NAME_PATTERN } from '@/Modules/Environments/Types/EnvironmentRules';

export type TTargetKind = TTarget['Kind'];

export interface IHostDraft {
  Host: string;
  Port: string;
}

export interface ITargetDraft {
  Name: string;
  Kind: TTargetKind;
  Username: string;
  DeployPath: string;
  KeepReleases: string;
  BatchSize: string;
  BatchPauseSeconds: string;
  AllowRoot: boolean;
  ReloadCommand: string;
  PreSwitchCommand: string;
  PostSwitchCommand: string;
  Shared: string;
  ProjectName: string;
  ComposeFile: string;
  AllowUnsafeCompose: boolean;
  AllowedImages: string;
  KeySecretName: string;
  Hosts: IHostDraft[];
}

export type TTargetProblem =
  | 'targets.form.error.name'
  | 'targets.form.error.username'
  | 'targets.form.error.root'
  | 'targets.form.error.path'
  | 'targets.form.error.keySecret'
  | 'targets.form.error.noHost'
  | 'targets.form.error.host'
  | 'targets.form.error.port'
  | 'targets.form.error.keep'
  | 'targets.form.error.batch'
  | 'targets.form.error.pause'
  | 'targets.form.error.shared'
  | 'targets.form.error.projectName';

export const MAX_HOSTS = 10;
export const DEPLOY_PATH_PATTERN = /^\/[A-Za-z0-9._/-]{1,200}$/;
export const SHARED_PATTERN = /^[A-Za-z0-9.][A-Za-z0-9._-]{0,63}$/;
export const PROJECT_NAME_PATTERN = /^[a-z0-9][a-z0-9_-]{0,62}$/;
export const SECRET_NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;
export const DEFAULT_KEEP_RELEASES = 5;
export const DEFAULT_PORT = '22';

export const EMPTY_HOST: IHostDraft = { Host: '', Port: DEFAULT_PORT };

export const EMPTY_TARGET_DRAFT: ITargetDraft = {
  Name: '',
  Kind: 'ssh',
  Username: 'deploy',
  DeployPath: '',
  KeepReleases: String(DEFAULT_KEEP_RELEASES),
  BatchSize: '1',
  BatchPauseSeconds: '0',
  AllowRoot: false,
  ReloadCommand: '',
  PreSwitchCommand: '',
  PostSwitchCommand: '',
  Shared: '',
  ProjectName: '',
  ComposeFile: '',
  AllowUnsafeCompose: false,
  AllowedImages: '',
  KeySecretName: '',
  Hosts: [EMPTY_HOST],
};

const linesOf = (text: string): string[] =>
  text
    .split(/[\n,]/)
    .map((line) => line.trim())
    .filter((line) => line !== '');

const whole = (text: string): number | null => (/^\d{1,6}$/.test(text.trim()) ? Number(text.trim()) : null);

export const draftOfTarget = (target: TTarget): ITargetDraft => ({
  Name: target.Name,
  Kind: target.Kind,
  Username: target.Username,
  DeployPath: target.DeployPath,
  KeepReleases: String(target.KeepReleases),
  BatchSize: String(target.Settings.Rolling?.BatchSize ?? 1),
  BatchPauseSeconds: String(target.Settings.Rolling?.BatchPauseSeconds ?? 0),
  AllowRoot: target.Settings.AllowRoot ?? false,
  ReloadCommand: target.Settings.Ssh?.ReloadCommand ?? '',
  PreSwitchCommand: target.Settings.Ssh?.PreSwitchCommand ?? '',
  PostSwitchCommand: target.Settings.Ssh?.PostSwitchCommand ?? '',
  Shared: (target.Settings.Ssh?.Shared ?? []).join('\n'),
  ProjectName: target.Settings.Compose?.ProjectName ?? '',
  ComposeFile: target.Settings.Compose?.ComposeFile ?? '',
  AllowUnsafeCompose: target.Settings.Compose?.AllowUnsafeCompose ?? false,
  AllowedImages: target.AllowedImages.join('\n'),
  KeySecretName: target.KeySecretName ?? '',
  Hosts: target.Hosts.map((host) => ({ Host: host.Host, Port: String(host.Port) })),
});

const sshSettings = (draft: ITargetDraft): NonNullable<TTargetSettings['Ssh']> => {
  const ssh: NonNullable<TTargetSettings['Ssh']> = {};
  if (draft.ReloadCommand.trim() !== '') ssh.ReloadCommand = draft.ReloadCommand.trim();
  if (draft.PreSwitchCommand.trim() !== '') ssh.PreSwitchCommand = draft.PreSwitchCommand.trim();
  if (draft.PostSwitchCommand.trim() !== '') ssh.PostSwitchCommand = draft.PostSwitchCommand.trim();
  const shared = linesOf(draft.Shared);
  if (shared.length > 0) ssh.Shared = shared;
  return ssh;
};

const composeSettings = (draft: ITargetDraft): NonNullable<TTargetSettings['Compose']> => {
  const compose: NonNullable<TTargetSettings['Compose']> = {};
  if (draft.ProjectName.trim() !== '') compose.ProjectName = draft.ProjectName.trim();
  if (draft.ComposeFile.trim() !== '') compose.ComposeFile = draft.ComposeFile.trim();
  if (draft.AllowUnsafeCompose) compose.AllowUnsafeCompose = true;
  return compose;
};

export const settingsOf = (draft: ITargetDraft): TTargetSettings => {
  const settings: TTargetSettings = {};
  if (draft.Hosts.length > 1) {
    settings.Rolling = { BatchSize: whole(draft.BatchSize) ?? 1, BatchPauseSeconds: whole(draft.BatchPauseSeconds) ?? 0 };
  }
  if (draft.AllowRoot) settings.AllowRoot = true;
  if (draft.Kind === 'ssh') {
    const ssh = sshSettings(draft);
    if (Object.keys(ssh).length > 0) settings.Ssh = ssh;
  } else {
    const compose = composeSettings(draft);
    if (Object.keys(compose).length > 0) settings.Compose = compose;
  }
  return settings;
};

export const updateInputOf = (draft: ITargetDraft): TUpdateTargetInput => ({
  Username: draft.Username.trim(),
  DeployPath: draft.DeployPath.trim(),
  KeepReleases: whole(draft.KeepReleases) ?? DEFAULT_KEEP_RELEASES,
  Settings: settingsOf(draft),
  AllowedImages: linesOf(draft.AllowedImages),
});

export const createInputOf = (draft: ITargetDraft): TCreateTargetInput => ({
  Name: draft.Name.trim(),
  Kind: draft.Kind,
  Username: draft.Username.trim(),
  DeployPath: draft.DeployPath.trim(),
  KeepReleases: whole(draft.KeepReleases) ?? DEFAULT_KEEP_RELEASES,
  Settings: settingsOf(draft),
  AllowedImages: linesOf(draft.AllowedImages),
  ...(draft.KeySecretName.trim() === '' ? {} : { KeySecretName: draft.KeySecretName.trim() }),
  Hosts: draft.Hosts.map((host) => ({ Host: host.Host.trim(), Port: whole(host.Port) ?? Number(DEFAULT_PORT) })),
});

export const validPort = (text: string): boolean => {
  const port = whole(text);
  return port !== null && port >= 1 && port <= 65535;
};

export const hostProblem = (host: IHostDraft): TTargetProblem | null => {
  if (host.Host.trim() === '' || /\s/.test(host.Host.trim())) return 'targets.form.error.host';
  return validPort(host.Port) ? null : 'targets.form.error.port';
};

const numberProblem = (draft: ITargetDraft): TTargetProblem | null => {
  const keep = whole(draft.KeepReleases);
  if (keep === null || keep < 2 || keep > 50) return 'targets.form.error.keep';
  if (draft.Hosts.length < 2) return null;
  const batch = whole(draft.BatchSize);
  if (batch === null || batch < 1 || batch > Math.min(10, draft.Hosts.length)) return 'targets.form.error.batch';
  const pause = whole(draft.BatchPauseSeconds);
  return pause === null || pause > 600 ? 'targets.form.error.pause' : null;
};

const settingsProblem = (draft: ITargetDraft): TTargetProblem | null => {
  if (draft.Kind === 'ssh') {
    return linesOf(draft.Shared).every((name) => SHARED_PATTERN.test(name)) ? null : 'targets.form.error.shared';
  }
  const project = draft.ProjectName.trim();
  return project === '' || PROJECT_NAME_PATTERN.test(project) ? null : 'targets.form.error.projectName';
};

export const draftProblem = (draft: ITargetDraft, creating: boolean): TTargetProblem | null => {
  if (creating && !NAME_PATTERN.test(draft.Name.trim())) return 'targets.form.error.name';
  if (draft.Username.trim() === '' || draft.Username.trim().length > 32) return 'targets.form.error.username';
  if (draft.Username.trim() === 'root' && !draft.AllowRoot) return 'targets.form.error.root';
  if (!DEPLOY_PATH_PATTERN.test(draft.DeployPath.trim())) return 'targets.form.error.path';
  if (draft.KeySecretName.trim() !== '' && !SECRET_NAME_PATTERN.test(draft.KeySecretName.trim())) return 'targets.form.error.keySecret';
  if (draft.Hosts.length < 1) return 'targets.form.error.noHost';
  for (const host of draft.Hosts) {
    const problem = hostProblem(host);
    if (problem !== null) return problem;
  }
  return numberProblem(draft) ?? settingsProblem(draft);
};

export const defaultKeySecretName = (targetName: string): string => `${targetName.toUpperCase().replace(/-/g, '_')}_SSH_KEY`;

export const FINGERPRINT_PATTERN = /^SHA256:[A-Za-z0-9+/]{43}$/;

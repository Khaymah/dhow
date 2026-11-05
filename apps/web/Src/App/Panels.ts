import { canCreateProjects, canManageMembers, canManageRunners, canManageTokens, type TRole } from '@/Core/Auth/AuthTypes';
import { isCustomThemeId } from '@/Core/Theme/ThemeIds';

export const UNKNOWN_SLIDE = 'unknown';

const STATIC_SLIDES = [
  'runner-new',
  'organization-invite',
  'organization-transfer',
  'organization-token-new',
  'organization-new',
  'team-new',
  'account-password',
  'account-totp',
  'account-token-new',
  'theme-edit',
  'member-remove',
  'organization-leave',
  'oidc-new',
  'project-new',
  'repository-link',
  'run-trigger',
  'secret-new',
  'variable-new',
  'registry-new',
  'environment-new',
] as const;
const THEME_EDIT_PREFIX = 'theme-edit-';
const OIDC_EDIT_PREFIX = 'oidc-edit-';
const SECRET_EDIT_PREFIX = 'secret-edit-';
const VARIABLE_EDIT_PREFIX = 'variable-edit-';
const REGISTRY_EDIT_PREFIX = 'registry-edit-';
const OIDC_ID = /^[a-z][a-z0-9]{0,31}$/;
const SECRET_NAME = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;
const CREDENTIAL_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export type TStaticSlideId = (typeof STATIC_SLIDES)[number];
export type TThemeEditSlideId = `theme-edit-${string}`;
export type TOidcEditSlideId = `oidc-edit-${string}`;
export type TSecretEditSlideId = `secret-edit-${string}`;
export type TVariableEditSlideId = `variable-edit-${string}`;
export type TRegistryEditSlideId = `registry-edit-${string}`;
export type TSlideId =
  | TStaticSlideId
  | TThemeEditSlideId
  | TOidcEditSlideId
  | TSecretEditSlideId
  | TVariableEditSlideId
  | TRegistryEditSlideId;

export type TSecretsEditKind = 'secret' | 'variable' | 'registry';

export interface ISecretsEditTarget {
  Kind: TSecretsEditKind;
  Key: string;
}

export interface IShellSearch {
  slide?: TSlideId | typeof UNKNOWN_SLIDE;
  stack?: string;
}

export const themeEditSlide = (id: string): TThemeEditSlideId => `${THEME_EDIT_PREFIX}${id}`;

export const oidcEditSlide = (id: string): TOidcEditSlideId => `${OIDC_EDIT_PREFIX}${id}`;

export const secretEditSlide = (name: string): TSecretEditSlideId => `${SECRET_EDIT_PREFIX}${name}`;

export const variableEditSlide = (name: string): TVariableEditSlideId => `${VARIABLE_EDIT_PREFIX}${name}`;

export const registryEditSlide = (id: string): TRegistryEditSlideId => `${REGISTRY_EDIT_PREFIX}${id}`;

const EDIT_TARGETS: readonly [string, TSecretsEditKind, RegExp][] = [
  [SECRET_EDIT_PREFIX, 'secret', SECRET_NAME],
  [VARIABLE_EDIT_PREFIX, 'variable', SECRET_NAME],
  [REGISTRY_EDIT_PREFIX, 'registry', CREDENTIAL_ID],
];

export const secretsTargetOf = (id: string): ISecretsEditTarget | null => {
  for (const [prefix, Kind, pattern] of EDIT_TARGETS) {
    if (id.startsWith(prefix) && pattern.test(id.slice(prefix.length))) return { Kind, Key: id.slice(prefix.length) };
  }
  return null;
};

export const oidcOf = (id: TSlideId): string | null => (id.startsWith(OIDC_EDIT_PREFIX) ? id.slice(OIDC_EDIT_PREFIX.length) : null);

export const isSlideId = (value: unknown): value is TSlideId => {
  if (typeof value !== 'string') return false;
  if (STATIC_SLIDES.some((id) => id === value)) return true;
  if (value.startsWith(OIDC_EDIT_PREFIX)) return OIDC_ID.test(value.slice(OIDC_EDIT_PREFIX.length));
  if (secretsTargetOf(value) !== null) return true;
  return value.startsWith(THEME_EDIT_PREFIX) && isCustomThemeId(value.slice(THEME_EDIT_PREFIX.length));
};

export const themeOf = (id: TSlideId): string | null =>
  id.startsWith(THEME_EDIT_PREFIX) ? id.slice(THEME_EDIT_PREFIX.length) : null;

export const isThemeEditSlide = (id: TSlideId): boolean => id === 'theme-edit' || id.startsWith(THEME_EDIT_PREFIX);

const SLIDE_GATES: Partial<Record<TStaticSlideId, (role: TRole | undefined) => boolean>> = {
  'runner-new': canManageRunners,
  'project-new': canCreateProjects,
  'repository-link': (role) => role !== undefined,
  'run-trigger': (role) => role !== undefined,
  'organization-invite': canManageMembers,
  'team-new': canManageMembers,
  'organization-token-new': canManageTokens,
  'organization-transfer': (role) => role === 'owner',
  'member-remove': canManageMembers,
  'organization-leave': (role) => role !== undefined,
  'secret-new': (role) => role !== undefined,
  'variable-new': (role) => role !== undefined,
  'registry-new': (role) => role !== undefined,
  'environment-new': (role) => role !== undefined,
};

const isStaticSlide = (id: TSlideId): id is TStaticSlideId => STATIC_SLIDES.some((slide) => slide === id);

export const slideAllowed = (id: TSlideId, role: TRole | undefined): boolean => {
  const gate = isStaticSlide(id) ? SLIDE_GATES[id] : undefined;
  return gate === undefined || gate(role);
};

const legacyStack = (raw: unknown): string | undefined => {
  if (raw === undefined || raw === '') return undefined;
  if (Array.isArray(raw)) return raw.map(String).join(',');
  return typeof raw === 'string' ? raw : JSON.stringify(raw);
};

export const parseShellSearch = (search: Record<string, unknown>): IShellSearch => {
  const result: IShellSearch = {};
  const stack = legacyStack(search.stack);
  if (stack !== undefined) result.stack = stack;
  const raw = search.slide;
  if (raw !== undefined && raw !== '') result.slide = isSlideId(raw) ? raw : UNKNOWN_SLIDE;
  return result;
};

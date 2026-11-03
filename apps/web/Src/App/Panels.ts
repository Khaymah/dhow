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
] as const;
const THEME_EDIT_PREFIX = 'theme-edit-';
const OIDC_EDIT_PREFIX = 'oidc-edit-';
const OIDC_ID = /^[a-z][a-z0-9]{0,31}$/;

export type TStaticSlideId = (typeof STATIC_SLIDES)[number];
export type TThemeEditSlideId = `theme-edit-${string}`;
export type TOidcEditSlideId = `oidc-edit-${string}`;
export type TSlideId = TStaticSlideId | TThemeEditSlideId | TOidcEditSlideId;

export interface IShellSearch {
  slide?: TSlideId | typeof UNKNOWN_SLIDE;
  stack?: string;
}

export const themeEditSlide = (id: string): TThemeEditSlideId => `${THEME_EDIT_PREFIX}${id}`;

export const oidcEditSlide = (id: string): TOidcEditSlideId => `${OIDC_EDIT_PREFIX}${id}`;

export const oidcOf = (id: TSlideId): string | null => (id.startsWith(OIDC_EDIT_PREFIX) ? id.slice(OIDC_EDIT_PREFIX.length) : null);

export const isSlideId = (value: unknown): value is TSlideId => {
  if (typeof value !== 'string') return false;
  if (STATIC_SLIDES.some((id) => id === value)) return true;
  if (value.startsWith(OIDC_EDIT_PREFIX)) return OIDC_ID.test(value.slice(OIDC_EDIT_PREFIX.length));
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

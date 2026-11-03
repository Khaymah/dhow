import type { components } from '@/Core/Api/Schema';

type TSchemas = components['schemas'];

export type TRole = TSchemas['Role'];
export type TBindableRole = TSchemas['BindableRole'];
export type TUser = TSchemas['User'];
export type TMembership = TSchemas['MembershipView'];
export type TOrganization = TSchemas['Organization'];
export type TSecondFactorRule = TSchemas['SecondFactorRule'];
export type TMe = Omit<TSchemas['Me'], 'CsrfToken'>;

export const ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer', 'viewer'];
export const BINDABLE_ROLES: readonly TBindableRole[] = ['admin', 'maintainer', 'developer', 'viewer'];
export const SECOND_FACTOR_RULES: readonly TSecondFactorRule[] = ['off', 'admins', 'everyone'];

export const ruleReach = (rule: TSecondFactorRule): number => SECOND_FACTOR_RULES.indexOf(rule);

const SECOND_FACTOR_ADMIN_ROLES: readonly TRole[] = ['owner', 'admin'];

export const secondFactorCovers = (rule: TSecondFactorRule, role: TRole): boolean =>
  rule === 'everyone' || (rule === 'admins' && SECOND_FACTOR_ADMIN_ROLES.includes(role));

const AUDIT_ROLES: readonly TRole[] = ['owner', 'admin'];
const ORGANIZATION_EDIT_ROLES: readonly TRole[] = ['owner', 'admin'];
const MEMBER_READ_ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer'];
const MEMBER_MANAGE_ROLES: readonly TRole[] = ['owner', 'admin'];
const TOKEN_MANAGE_ROLES: readonly TRole[] = ['owner', 'admin'];
const RUNNER_READ_ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer'];
const RUNNER_MANAGE_ROLES: readonly TRole[] = ['owner', 'admin'];
const PROJECT_CREATE_ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer'];

export const canReadAudit = (role: TRole | undefined): boolean => role !== undefined && AUDIT_ROLES.includes(role);

export const canEditOrganization = (role: TRole | undefined): boolean =>
  role !== undefined && ORGANIZATION_EDIT_ROLES.includes(role);

export const canReadMembers = (role: TRole | undefined): boolean => role !== undefined && MEMBER_READ_ROLES.includes(role);

export const canManageMembers = (role: TRole | undefined): boolean =>
  role !== undefined && MEMBER_MANAGE_ROLES.includes(role);

export const canManageTokens = (role: TRole | undefined): boolean => role !== undefined && TOKEN_MANAGE_ROLES.includes(role);

export const canReadRunners = (role: TRole | undefined): boolean => role !== undefined && RUNNER_READ_ROLES.includes(role);

export const canManageRunners = (role: TRole | undefined): boolean =>
  role !== undefined && RUNNER_MANAGE_ROLES.includes(role);

export const canCreateProjects = (role: TRole | undefined): boolean =>
  role !== undefined && PROJECT_CREATE_ROLES.includes(role);

export const rankOf = (role: TRole | undefined): number => (role === undefined ? 0 : ROLES.length - ROLES.indexOf(role));

export const canGrant = (actor: TRole | undefined, role: TRole): boolean => rankOf(actor) >= rankOf(role);

export const canManage = (actor: TRole | undefined, target: TRole): boolean =>
  target === 'owner' ? actor === 'owner' : canManageMembers(actor) && canGrant(actor, target);

export const grantableRoles = (actor: TRole | undefined): TBindableRole[] =>
  BINDABLE_ROLES.filter((role) => canGrant(actor, role));

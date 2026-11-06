import type { TRole } from '@/Core/Auth/AuthTypes';

const MANAGE_ROLES: readonly TRole[] = ['owner', 'admin'];
const PAUSE_ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer'];
const SECRET_READ_ROLES: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer'];

export const canManageEnvironments = (role: TRole | undefined): boolean => role !== undefined && MANAGE_ROLES.includes(role);

export const canPauseEnvironments = (role: TRole | undefined): boolean => role !== undefined && PAUSE_ROLES.includes(role);

export const canReadEnvironmentSecrets = (role: TRole | undefined): boolean => role !== undefined && SECRET_READ_ROLES.includes(role);

export const canActOnTargets = (role: TRole | undefined): boolean => role !== undefined && PAUSE_ROLES.includes(role);

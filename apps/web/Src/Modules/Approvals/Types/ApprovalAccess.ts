import type { TRole } from '@/Core/Auth/AuthTypes';

const BYPASS_ROLES: readonly TRole[] = ['owner', 'admin'];

export const COMMENT_MAX_BYTES = 1024;
export const REASON_MAX_BYTES = 1024;

export const canBypassDeployments = (role: TRole | undefined): boolean => role !== undefined && BYPASS_ROLES.includes(role);

export const canOpenEnvironmentRules = canBypassDeployments;

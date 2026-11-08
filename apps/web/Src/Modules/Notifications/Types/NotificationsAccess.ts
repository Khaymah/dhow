import type { TRole } from '@/Core/Auth/AuthTypes';

const MANAGERS: readonly TRole[] = ['owner', 'admin', 'maintainer'];

export const canManageNotifications = (role: TRole | undefined): boolean => role !== undefined && MANAGERS.includes(role);

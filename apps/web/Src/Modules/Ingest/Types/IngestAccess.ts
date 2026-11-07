import type { TRole } from '@/Core/Auth/AuthTypes';

const MANAGERS: readonly TRole[] = ['owner', 'admin', 'maintainer'];
const READERS: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer'];

export const canManageIngest = (role: TRole | undefined): boolean => role !== undefined && MANAGERS.includes(role);

export const canConfirmProtectedReach = (role: TRole | undefined): boolean => role === 'owner' || role === 'admin';

export const canReadIngested = (role: TRole | undefined): boolean => role !== undefined && READERS.includes(role);

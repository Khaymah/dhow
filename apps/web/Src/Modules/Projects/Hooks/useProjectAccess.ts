import type { TRole } from '@/Core/Auth/AuthTypes';
import { useCurrentMembership, useMe } from '@/Core/Auth/useMe';

export type TOrganizationAccess = { Kind: 'loading' } | { Kind: 'denied' } | { Kind: 'allowed'; OrganizationId: string; Role: TRole };

export const useOrganizationAccess = (): TOrganizationAccess => {
  const me = useMe();
  const membership = useCurrentMembership();
  if (membership === undefined) return me.isPending ? { Kind: 'loading' } : { Kind: 'denied' };
  return { Kind: 'allowed', OrganizationId: membership.OrganizationId, Role: membership.Role };
};

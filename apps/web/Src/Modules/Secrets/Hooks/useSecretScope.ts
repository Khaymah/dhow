import { canManageOrganizationSecrets } from '@/Core/Auth/AuthTypes';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import type { ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';

export type TSecretScopeAccess = { Kind: 'loading' } | { Kind: 'denied' } | { Kind: 'allowed'; Scope: ISecretScope };

export const useSecretScope = (projectId: string | undefined): TSecretScopeAccess => {
  const membership = useCurrentMembership();
  if (membership === undefined) return { Kind: 'loading' };
  if (projectId === undefined && !canManageOrganizationSecrets(membership.Role)) return { Kind: 'denied' };
  return {
    Kind: 'allowed',
    Scope: {
      OrganizationId: membership.OrganizationId,
      ProjectId: projectId ?? null,
    },
  };
};

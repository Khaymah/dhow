import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import {
  listProjectChoices,
  listRegistryCredentials,
  listSecrets,
  listVariables,
  projectChoicesKey,
  registryKey,
  secretsKey,
  variablesKey,
  type ISecretScope,
  type TOrganizationProject,
  type TRegistryCredential,
  type TSecret,
  type TVariable,
} from '@/Modules/Secrets/Api/SecretsApi';

export const useSecrets = (scope: ISecretScope): UseQueryResult<TSecret[], ApiError> =>
  useQuery<TSecret[], ApiError>({
    queryKey: secretsKey(scope),
    queryFn: () => listSecrets(scope),
  });

export const useVariables = (scope: ISecretScope): UseQueryResult<TVariable[], ApiError> =>
  useQuery<TVariable[], ApiError>({
    queryKey: variablesKey(scope),
    queryFn: () => listVariables(scope),
  });

export const useRegistryCredentials = (scope: ISecretScope): UseQueryResult<TRegistryCredential[], ApiError> =>
  useQuery<TRegistryCredential[], ApiError>({
    queryKey: registryKey(scope),
    queryFn: () => listRegistryCredentials(scope),
  });

export const useProjectChoices = (organizationId: string): UseQueryResult<TOrganizationProject[], ApiError> =>
  useQuery<TOrganizationProject[], ApiError>({
    queryKey: projectChoicesKey(organizationId),
    queryFn: () => listProjectChoices(organizationId),
  });

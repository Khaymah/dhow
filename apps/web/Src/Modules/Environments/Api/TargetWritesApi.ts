import { ApiClient, request } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';

type TSchemas = components['schemas'];

export type TCreateTargetInput = Omit<TSchemas['CreateDeployTargetRequest'], 'CurrentFactor'>;
export type TUpdateTargetInput = Omit<TSchemas['UpdateDeployTargetRequest'], 'CurrentFactor' | 'Version'>;
export type TTargetSettings = TSchemas['DeployTargetSettings'];

export interface ITargetScope {
  OrganizationId: string;
  ProjectId: string;
  EnvironmentName: string;
}

const base = (scope: ITargetScope) => ({ id: scope.OrganizationId, projectId: scope.ProjectId, environmentName: scope.EnvironmentName });
const named = (scope: ITargetScope, targetName: string) => ({ ...base(scope), targetName });
const hosted = (scope: ITargetScope, targetName: string, hostId: string) => ({ ...named(scope, targetName), hostId });

export const createTarget = (scope: ITargetScope, input: TCreateTargetInput, factor: TCurrentFactor): Promise<TTarget> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets', {
      params: { path: base(scope) },
      body: { ...input, CurrentFactor: factor },
    }),
  );

export const updateTarget = (
  scope: ITargetScope,
  targetName: string,
  version: number,
  input: TUpdateTargetInput,
  factor: TCurrentFactor,
): Promise<TTarget> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}', {
      params: { path: named(scope, targetName) },
      body: { ...input, Version: version, CurrentFactor: factor },
    }),
  );

export const removeTarget = (scope: ITargetScope, targetName: string, factor: TCurrentFactor): Promise<TTarget> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/remove', {
      params: { path: named(scope, targetName) },
      body: { CurrentFactor: factor },
    }),
  );

export const generateTargetKey = (scope: ITargetScope, targetName: string, factor: TCurrentFactor): Promise<TTarget> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/generate-key', {
      params: { path: named(scope, targetName) },
      body: { CurrentFactor: factor },
    }),
  );

export const addTargetHost = (
  scope: ITargetScope,
  targetName: string,
  version: number,
  host: { Host: string; Port?: number },
  factor: TCurrentFactor,
): Promise<TTarget> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts', {
      params: { path: named(scope, targetName) },
      body: { ...host, Version: version, CurrentFactor: factor },
    }),
  );

export const updateTargetHost = (
  scope: ITargetScope,
  targetName: string,
  hostId: string,
  version: number,
  host: { Host: string; Port?: number },
  factor: TCurrentFactor,
): Promise<TTarget> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts/{hostId}', {
      params: { path: hosted(scope, targetName, hostId) },
      body: { ...host, Version: version, CurrentFactor: factor },
    }),
  );

export const removeTargetHost = (
  scope: ITargetScope,
  targetName: string,
  hostId: string,
  version: number,
  factor: TCurrentFactor,
): Promise<TTarget> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts/{hostId}/remove', {
      params: { path: hosted(scope, targetName, hostId) },
      body: { Version: version, CurrentFactor: factor },
    }),
  );

export const reorderTargetHosts = (
  scope: ITargetScope,
  targetName: string,
  version: number,
  hostIds: string[],
  factor: TCurrentFactor,
): Promise<TTarget> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts-order', {
      params: { path: named(scope, targetName) },
      body: { Version: version, HostIds: hostIds, CurrentFactor: factor },
    }),
  );

export const fetchHostKey = (scope: ITargetScope, targetName: string, hostId: string): Promise<TTarget> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts/{hostId}/fetch-host-key', {
      params: { path: hosted(scope, targetName, hostId) },
    }),
  );

export const pasteHostKey = (scope: ITargetScope, targetName: string, hostId: string, hostKey: string): Promise<TTarget> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts/{hostId}/pending-host-key', {
      params: { path: hosted(scope, targetName, hostId) },
      body: { HostKey: hostKey },
    }),
  );

export const confirmHostKey = (
  scope: ITargetScope,
  targetName: string,
  hostId: string,
  fingerprint: string,
  factor: TCurrentFactor,
): Promise<TTarget> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts/{hostId}/confirm-host-key', {
      params: { path: hosted(scope, targetName, hostId) },
      body: { Fingerprint: fingerprint, CurrentFactor: factor },
    }),
  );

export const putTargetKeySecret = (scope: ITargetScope, secretName: string, value: string): Promise<TSchemas['Secret']> =>
  request(() =>
    ApiClient.PUT('/organizations/{id}/projects/{projectId}/environments/{environmentName}/secrets/{secretName}', {
      params: { path: { ...base(scope), secretName } },
      body: { Value: value },
    }),
  );

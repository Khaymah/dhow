import { ApiClient, request } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';
import type { TCurrentFactor } from '@/Core/Auth/Factor';

type TSchemas = components['schemas'];

export type TApproval = TSchemas['DeploymentApproval'];
export type TApprovalState = TSchemas['DeploymentApprovalState'];

export const approvalsKey = (organizationId: string) => ['approvals', organizationId] as const;
export const runApprovalsKey = (organizationId: string, projectId: string, runId: string) =>
  [...approvalsKey(organizationId), 'run', projectId, runId] as const;
export const myApprovalsKey = (organizationId: string) => [...approvalsKey(organizationId), 'mine'] as const;
export const bypassKey = (organizationId: string, projectId: string, environment: string) =>
  [...approvalsKey(organizationId), 'bypass', projectId, environment] as const;

const approvalPath = (id: string, projectId: string, runId: string, approvalId: string) => ({ id, projectId, runId, approvalId });

export const listRunApprovals = (organizationId: string, projectId: string, runId: string): Promise<TApproval[]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/runs/{runId}/approvals', {
      params: { path: { id: organizationId, projectId, runId } },
    }),
  );

export const listMyApprovals = (organizationId: string): Promise<TApproval[]> =>
  request(() =>
    ApiClient.GET('/organizations/{id}/deployment-approvals', {
      params: { path: { id: organizationId }, query: { Mine: true } },
    }),
  );

export const readBypassAllowed = async (organizationId: string, projectId: string, environment: string): Promise<boolean> => {
  const found = await request(() =>
    ApiClient.GET('/organizations/{id}/projects/{projectId}/environments/{environmentName}', {
      params: { path: { id: organizationId, projectId, environmentName: environment } },
    }),
  );
  return found.Rules.AllowAdminBypass;
};

export const approveDeployment = (
  organizationId: string,
  projectId: string,
  runId: string,
  approvalId: string,
  factor: TCurrentFactor,
  comment: string,
): Promise<TApproval> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/runs/{runId}/approvals/{approvalId}/approve', {
      params: { path: approvalPath(organizationId, projectId, runId, approvalId) },
      body: { CurrentFactor: factor, ...(comment === '' ? {} : { Comment: comment }) },
    }),
  );

export const rejectDeployment = (
  organizationId: string,
  projectId: string,
  runId: string,
  approvalId: string,
  factor: TCurrentFactor,
  comment: string,
): Promise<TApproval> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/runs/{runId}/approvals/{approvalId}/reject', {
      params: { path: approvalPath(organizationId, projectId, runId, approvalId) },
      body: { CurrentFactor: factor, ...(comment === '' ? {} : { Comment: comment }) },
    }),
  );

export const bypassDeployment = (
  organizationId: string,
  projectId: string,
  runId: string,
  approvalId: string,
  factor: TCurrentFactor,
  reason: string,
): Promise<TApproval> =>
  request(() =>
    ApiClient.POST('/organizations/{id}/projects/{projectId}/runs/{runId}/approvals/{approvalId}/bypass', {
      params: { path: approvalPath(organizationId, projectId, runId, approvalId) },
      body: { CurrentFactor: factor, Reason: reason },
    }),
  );

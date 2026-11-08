import { ApiClient, requestConditional, type IConditional } from '@/Core/Api/Client';
import type { components } from '@/Core/Api/Schema';

type TSchemas = components['schemas'];

export type TDeployBoard = TSchemas['DeployBoard'];
export type TBoardEnvironment = TSchemas['DeployBoardEnvironment'];
export type TBoardTarget = TSchemas['DeployBoardTarget'];
export type TBoardRelease = TSchemas['DeployBoardRelease'];
export type TBoardProgress = TSchemas['DeployBoardProgress'];
export type TBoardReleaseState = TBoardRelease['State'];

export const boardsKey = (organizationId: string) => ['boards', organizationId] as const;
export const boardKey = (organizationId: string, projectId: string) => [...boardsKey(organizationId), projectId] as const;

export const getDeployBoard = (
  organizationId: string,
  projectId: string,
  previous: IConditional<TDeployBoard> | undefined,
): Promise<IConditional<TDeployBoard>> =>
  requestConditional(
    (etag) =>
      ApiClient.GET('/organizations/{id}/projects/{projectId}/deploy-board', {
        params: { path: { id: organizationId, projectId }, header: etag === undefined ? {} : { 'If-None-Match': etag } },
      }),
    previous,
  );

import { useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import type { IConditional } from '@/Core/Api/Client';
import { boardKey, getDeployBoard, type TDeployBoard } from '@/Modules/Boards/Api/BoardsApi';

export const useDeployBoard = (organizationId: string, projectId: string): UseQueryResult<TDeployBoard, ApiError> => {
  const client = useQueryClient();
  return useQuery<IConditional<TDeployBoard>, ApiError, TDeployBoard>({
    queryKey: boardKey(organizationId, projectId),
    queryFn: () => getDeployBoard(organizationId, projectId, client.getQueryData<IConditional<TDeployBoard>>(boardKey(organizationId, projectId))),
    select: (conditional) => conditional.Data,
  });
};

import { useQueryClient } from '@tanstack/react-query';

import { deploymentsKey } from '@/Modules/Environments/Api/EnvironmentsApi';
import { targetsKey } from '@/Modules/Environments/Api/TargetsApi';

export interface ITargetRefresh {
  Changed: () => void;
}

export const useTargetRefresh = (organizationId: string, projectId: string, environmentName: string): ITargetRefresh => {
  const client = useQueryClient();
  return {
    Changed: () => {
      void client.invalidateQueries({ queryKey: targetsKey(organizationId, projectId, environmentName) });
      void client.invalidateQueries({ queryKey: deploymentsKey(organizationId, projectId, environmentName) });
    },
  };
};

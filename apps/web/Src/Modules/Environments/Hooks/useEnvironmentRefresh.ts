import { useQueryClient } from '@tanstack/react-query';

import { environmentKey, environmentListKey, type TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';

export interface IEnvironmentRefresh {
  Saved: (environment: TEnvironment) => void;
  Removed: (environment: TEnvironment) => void;
}

export const useEnvironmentRefresh = (organizationId: string, projectId: string): IEnvironmentRefresh => {
  const client = useQueryClient();
  return {
    Saved: (environment) => {
      client.setQueryData(environmentKey(organizationId, projectId, environment.Name), environment);
      void client.invalidateQueries({ queryKey: environmentListKey(organizationId, projectId) });
    },
    Removed: (environment) => {
      void client.invalidateQueries({ queryKey: environmentKey(organizationId, projectId, environment.Name), refetchType: 'none' });
      void client.invalidateQueries({ queryKey: environmentListKey(organizationId, projectId) });
    },
  };
};

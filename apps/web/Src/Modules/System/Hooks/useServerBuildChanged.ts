import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { getMeta } from '@/Modules/System/Api/SystemApi';
import { META_QUERY_KEY } from '@/Modules/System/Hooks/useMeta';
import type { TMeta, TServerBuild } from '@/Modules/System/Types/SystemTypes';

export const BUILD_CHECK_INTERVAL_MS = 300_000;

export const sameBuild = (a: TServerBuild, b: TServerBuild): boolean =>
  a.Version === b.Version && a.Commit === b.Commit && a.BuiltAt === b.BuiltAt;

const buildOf = (meta: TMeta): TServerBuild | undefined => meta.ServerBuild;

export const useServerBuildChanged = (): boolean => {
  const { data: current } = useQuery({
    queryKey: META_QUERY_KEY,
    queryFn: getMeta,
    refetchInterval: BUILD_CHECK_INTERVAL_MS,
    select: buildOf,
  });
  const [Loaded, setLoaded] = useState<TServerBuild | undefined>(undefined);

  if (Loaded === undefined && current !== undefined) setLoaded(current);

  return Loaded !== undefined && current !== undefined && !sameBuild(Loaded, current);
};

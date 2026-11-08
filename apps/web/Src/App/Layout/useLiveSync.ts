import { useQueryClient, type QueryKey } from '@tanstack/react-query';
import { useCallback, useEffect, useRef } from 'react';

import { queryKeysFor } from '@/App/Navigation/LiveKeys';
import { ME_QUERY_KEY } from '@/Core/Auth/MeQuery';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import type { TOrgSignal } from '@/Core/Events/OrgEvents';
import { useOrgEvents } from '@/Core/Hooks/useOrgEvents';

export const LIVE_FLUSH_MS = 200;

export const useLiveSync = (): void => {
  const client = useQueryClient();
  const organizationId = useCurrentMembership()?.OrganizationId;
  const pending = useRef(new Map<string, QueryKey>());
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const Flush = useCallback(() => {
    timer.current = null;
    const keys = [...pending.current.values()];
    pending.current.clear();
    for (const queryKey of keys) void client.invalidateQueries({ queryKey });
  }, [client]);

  const Handle = useCallback(
    (signal: TOrgSignal) => {
      if (organizationId === undefined) return;
      if (signal.Kind === 'auth') {
        void client.invalidateQueries({ queryKey: ME_QUERY_KEY });
        return;
      }
      for (const key of queryKeysFor(organizationId, signal)) pending.current.set(JSON.stringify(key), key);
      timer.current ??= setTimeout(Flush, LIVE_FLUSH_MS);
    },
    [organizationId, Flush, client],
  );

  useOrgEvents(organizationId, Handle);

  useEffect(
    () => () => {
      if (timer.current !== null) clearTimeout(timer.current);
      timer.current = null;
      pending.current.clear();
    },
    [],
  );
};

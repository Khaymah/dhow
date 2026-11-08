import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react';

import type { TOrgEventsMode, TOrgSignalListener } from '@/Core/Events/OrgEvents';
import { orgEventsRegistry } from '@/Core/Events/OrgEventsRegistry';

export const useOrgEvents = (organizationId: string | undefined, listener: TOrgSignalListener): void => {
  const latest = useRef(listener);

  useEffect(() => {
    latest.current = listener;
  }, [listener]);

  useEffect(() => {
    if (organizationId === undefined) return undefined;
    const held = orgEventsRegistry.Acquire(organizationId);
    const off = held.Connection.Subscribe((signal) => {
      latest.current(signal);
    });
    return () => {
      off();
      held.Release();
    };
  }, [organizationId]);
};

export const useOrgEventsMode = (organizationId: string | undefined): TOrgEventsMode => {
  const subscribe = useCallback(
    (notify: () => void) => {
      if (organizationId === undefined) return () => undefined;
      const held = orgEventsRegistry.Acquire(organizationId);
      const off = held.Connection.SubscribeMode(notify);
      notify();
      return () => {
        off();
        held.Release();
      };
    },
    [organizationId],
  );
  const snapshot = (): TOrgEventsMode => (organizationId === undefined ? 'connecting' : (orgEventsRegistry.Peek(organizationId) ?? 'connecting'));
  return useSyncExternalStore(subscribe, snapshot, snapshot);
};

import { OrgEventsConnection, type TSourceFactory } from '@/Core/Events/OrgEventsConnection';

export const CLOSE_GRACE_MS = 1000;

interface IEntry {
  Connection: OrgEventsConnection;
  Users: number;
  CloseTimer: ReturnType<typeof setTimeout> | null;
}

export class OrgEventsRegistry {
  private readonly entries = new Map<string, IEntry>();

  constructor(private readonly create?: TSourceFactory) {}

  readonly Acquire = (organizationId: string): { Connection: OrgEventsConnection; Release: () => void } => {
    let entry = this.entries.get(organizationId);
    if (entry === undefined) {
      entry = { Connection: new OrgEventsConnection(organizationId, this.create), Users: 0, CloseTimer: null };
      this.entries.set(organizationId, entry);
    }
    if (entry.CloseTimer !== null) clearTimeout(entry.CloseTimer);
    entry.CloseTimer = null;
    entry.Users += 1;
    entry.Connection.Start();
    const held = entry;
    let released = false;
    return {
      Connection: held.Connection,
      Release: () => {
        if (released) return;
        released = true;
        held.Users -= 1;
        if (held.Users > 0) return;
        held.CloseTimer = setTimeout(() => {
          if (held.Users > 0) return;
          held.Connection.Stop();
          if (this.entries.get(organizationId) === held) this.entries.delete(organizationId);
        }, CLOSE_GRACE_MS);
      },
    };
  };

  readonly Reset = (): void => {
    for (const entry of this.entries.values()) {
      if (entry.CloseTimer !== null) clearTimeout(entry.CloseTimer);
      entry.Connection.Stop();
    }
    this.entries.clear();
  };

  readonly Size = (): number => this.entries.size;

  readonly Peek = (organizationId: string) => this.entries.get(organizationId)?.Connection.GetMode();
}

export const orgEventsRegistry = new OrgEventsRegistry();

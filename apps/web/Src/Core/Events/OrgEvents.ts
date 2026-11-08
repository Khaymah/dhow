export interface IOrgEvent {
  Seq: number;
  Type: string;
  Status?: string;
  ProjectId?: string;
  EnvironmentId?: string;
  TargetId?: string;
  SubjectId?: string;
  At: string;
}

export type TOrgSignal = { Kind: 'event'; Event: IOrgEvent } | { Kind: 'reset' } | { Kind: 'poll' } | { Kind: 'auth' };

export type TOrgEventsMode = 'connecting' | 'live' | 'polling' | 'stopped';

export type TOrgSignalListener = (signal: TOrgSignal) => void;

export const orgEventsUrl = (organizationId: string): string => `/api/v1/organizations/${organizationId}/events`;

export const parseOrgEvent = (raw: unknown): IOrgEvent | null => {
  if (typeof raw !== 'string') return null;
  try {
    const value: unknown = JSON.parse(raw);
    if (typeof value !== 'object' || value === null) return null;
    const event = value as Partial<IOrgEvent>;
    return typeof event.Type === 'string' && typeof event.Seq === 'number' ? (event as IOrgEvent) : null;
  } catch {
    return null;
  }
};

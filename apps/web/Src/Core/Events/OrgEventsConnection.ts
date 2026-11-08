import {
  orgEventsUrl,
  parseOrgEvent,
  type TOrgEventsMode,
  type TOrgSignal,
  type TOrgSignalListener,
} from '@/Core/Events/OrgEvents';

export const BACKOFF_MS = [1000, 2000, 4000, 8000, 15000] as const;
export const OPEN_WATCHDOG_MS = 10_000;
export const FALLBACK_AFTER_FAILURES = 3;
export const POLL_MS = 5000;
export const RETRY_STREAM_MS = 60_000;
const SOURCE_CLOSED = 2;

export type TSourceFactory = (url: string) => EventSource | null;

export type TProbe = (url: string) => Promise<number>;

export const browserProbe: TProbe = async (url) => {
  try {
    const response = await fetch(url, { credentials: 'same-origin', headers: { 'Last-Event-ID': 'probe' } });
    return response.status;
  } catch {
    return 0;
  }
};

export const browserSource: TSourceFactory = (url) =>
  typeof EventSource === 'undefined' ? null : new EventSource(url, { withCredentials: true });

export class OrgEventsConnection {
  private mode: TOrgEventsMode = 'connecting';
  private readonly listeners = new Set<TOrgSignalListener>();
  private readonly modeListeners = new Set<() => void>();
  private source: EventSource | null = null;
  private failures = 0;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private watchdog: ReturnType<typeof setTimeout> | null = null;
  private pollTimer: ReturnType<typeof setInterval> | null = null;
  private running = false;

  constructor(
    private readonly organizationId: string,
    private readonly create: TSourceFactory = browserSource,
    private readonly probe: TProbe = browserProbe,
  ) {}

  readonly GetMode = (): TOrgEventsMode => this.mode;

  readonly Subscribe = (listener: TOrgSignalListener): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  readonly SubscribeMode = (listener: () => void): (() => void) => {
    this.modeListeners.add(listener);
    return () => {
      this.modeListeners.delete(listener);
    };
  };

  Start(): void {
    if (this.running) return;
    this.running = true;
    this.failures = 0;
    this.setMode('connecting');
    document.addEventListener('visibilitychange', this.HandleVisibility);
    this.connect();
  }

  Stop(): void {
    this.running = false;
    document.removeEventListener('visibilitychange', this.HandleVisibility);
    this.closeSource();
    this.clearTimers();
  }

  private readonly HandleVisibility = (): void => {
    if (this.mode === 'polling' && document.visibilityState === 'visible') this.emit({ Kind: 'poll' });
  };

  private emit(signal: TOrgSignal): void {
    for (const listener of [...this.listeners]) listener(signal);
  }

  private setMode(mode: TOrgEventsMode): void {
    if (this.mode === mode) return;
    this.mode = mode;
    for (const listener of [...this.modeListeners]) listener();
  }

  private clearTimers(): void {
    if (this.retryTimer !== null) clearTimeout(this.retryTimer);
    if (this.watchdog !== null) clearTimeout(this.watchdog);
    if (this.pollTimer !== null) clearInterval(this.pollTimer);
    this.retryTimer = null;
    this.watchdog = null;
    this.pollTimer = null;
  }

  private closeSource(): void {
    this.source?.close();
    this.source = null;
  }

  private armWatchdog(): void {
    if (this.watchdog !== null) return;
    this.watchdog = setTimeout(() => {
      this.watchdog = null;
      this.failStream();
    }, OPEN_WATCHDOG_MS);
  }

  private disarmWatchdog(): void {
    if (this.watchdog !== null) clearTimeout(this.watchdog);
    this.watchdog = null;
  }

  private connect(): void {
    this.retryTimer = null;
    if (!this.running) return;
    const source = this.create(orgEventsUrl(this.organizationId));
    if (source === null) {
      this.failures = FALLBACK_AFTER_FAILURES;
      this.startPolling();
      this.scheduleRetry();
      return;
    }
    this.source = source;
    source.addEventListener('change', this.HandleChange);
    source.addEventListener('reset', this.HandleReset);
    source.onopen = this.HandleOpen;
    source.onerror = this.HandleError;
    this.armWatchdog();
  }

  private readonly HandleOpen = (): void => {
    this.disarmWatchdog();
    const wasPolling = this.mode === 'polling';
    this.failures = 0;
    if (this.pollTimer !== null) clearInterval(this.pollTimer);
    this.pollTimer = null;
    this.setMode('live');
    if (wasPolling) this.emit({ Kind: 'reset' });
  };

  private readonly HandleError = (): void => {
    if (this.source === null) return;
    if (this.source.readyState === SOURCE_CLOSED) {
      this.failStream();
      this.checkTerminal();
      return;
    }
    if (this.mode === 'live') this.setMode('connecting');
    this.armWatchdog();
  };

  private readonly HandleChange = (event: Event): void => {
    const parsed = parseOrgEvent(event instanceof MessageEvent ? event.data : null);
    if (parsed !== null) this.emit({ Kind: 'event', Event: parsed });
  };

  private readonly HandleReset = (): void => {
    this.emit({ Kind: 'reset' });
  };

  private checkTerminal(): void {
    void this.probe(orgEventsUrl(this.organizationId)).then((status) => {
      if (!this.running || (status !== 401 && status !== 403)) return;
      this.Stop();
      this.setMode('stopped');
      this.emit({ Kind: 'auth' });
    });
  }

  private failStream(): void {
    this.closeSource();
    this.disarmWatchdog();
    this.failures += 1;
    if (this.failures >= FALLBACK_AFTER_FAILURES) this.startPolling();
    else this.setMode('connecting');
    this.scheduleRetry();
  }

  private startPolling(): void {
    this.setMode('polling');
    this.pollTimer ??= setInterval(() => {
      if (document.visibilityState === 'visible') this.emit({ Kind: 'poll' });
    }, POLL_MS);
  }

  private scheduleRetry(): void {
    if (!this.running || this.retryTimer !== null) return;
    const delay = this.mode === 'polling' ? RETRY_STREAM_MS : (BACKOFF_MS[Math.min(this.failures, BACKOFF_MS.length) - 1] ?? BACKOFF_MS[0]);
    this.retryTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }
}

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { orgEventsUrl, parseOrgEvent, type TOrgSignal } from '@/Core/Events/OrgEvents';
import {
  BACKOFF_MS,
  FALLBACK_AFTER_FAILURES,
  OPEN_WATCHDOG_MS,
  OrgEventsConnection,
  POLL_MS,
  RETRY_STREAM_MS,
} from '@/Core/Events/OrgEventsConnection';
import { CLOSE_GRACE_MS, OrgEventsRegistry } from '@/Core/Events/OrgEventsRegistry';
import { FakeEventSource } from '@/Core/Testing/FakeEventSource';

const ORG = '0198a000-0000-7000-8000-0000000000a1';

const make = () => {
  FakeEventSource.instances = [];
  const connection = new OrgEventsConnection(ORG, (url) => new FakeEventSource(url, { withCredentials: true }) as unknown as EventSource);
  const seen: TOrgSignal[] = [];
  connection.Subscribe((signal) => seen.push(signal));
  return { connection, seen };
};

const event = (Seq: number, Type = 'deployment.finished') => ({ Seq, Type, Status: 'succeeded', ProjectId: 'p', At: '2026-11-07T10:00:00Z' });

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
});

afterEach(() => {
  vi.useRealTimers();
});

describe('the org event stream', () => {
  it('opens one credentialed stream for the organization and goes live when it opens', () => {
    const { connection } = make();
    connection.Start();
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(FakeEventSource.latest().url).toBe(orgEventsUrl(ORG));
    expect(FakeEventSource.latest().withCredentials).toBe(true);
    expect(connection.GetMode()).toBe('connecting');
    FakeEventSource.latest().open();
    expect(connection.GetMode()).toBe('live');
    connection.Stop();
  });

  it('hands each event to every subscriber and ignores what is not an event', () => {
    const { connection, seen } = make();
    const second: TOrgSignal[] = [];
    connection.Subscribe((signal) => second.push(signal));
    connection.Start();
    FakeEventSource.latest().open();
    FakeEventSource.latest().emit('change', event(1), '1-1');
    FakeEventSource.latest().emit('change', 'not json');
    FakeEventSource.latest().emit('change', { nothing: true });
    expect(seen).toEqual([{ Kind: 'event', Event: event(1) }]);
    expect(second).toEqual(seen);
    connection.Stop();
  });

  it('turns the reset event into a reset signal', () => {
    const { connection, seen } = make();
    connection.Start();
    FakeEventSource.latest().open();
    FakeEventSource.latest().emit('reset', { Epoch: 5 });
    expect(seen).toEqual([{ Kind: 'reset' }]);
    connection.Stop();
  });

  it('lets the browser reconnect on its own and says it is connecting meanwhile', () => {
    const { connection } = make();
    connection.Start();
    FakeEventSource.latest().open();
    FakeEventSource.latest().fail(false);
    expect(connection.GetMode()).toBe('connecting');
    expect(FakeEventSource.instances).toHaveLength(1);
    FakeEventSource.latest().open();
    expect(connection.GetMode()).toBe('live');
    connection.Stop();
  });

});

describe('the fallback of the org event stream', () => {
  it('retries a closed stream with a growing wait and falls back to polling after three failures', () => {
    const { connection, seen } = make();
    connection.Start();
    FakeEventSource.latest().fail(true);
    expect(connection.GetMode()).toBe('connecting');
    vi.advanceTimersByTime(BACKOFF_MS[0] - 1);
    expect(FakeEventSource.instances).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(FakeEventSource.instances).toHaveLength(2);

    FakeEventSource.latest().fail(true);
    vi.advanceTimersByTime(BACKOFF_MS[1]);
    expect(FakeEventSource.instances).toHaveLength(3);
    expect(connection.GetMode()).toBe('connecting');

    FakeEventSource.latest().fail(true);
    expect(FALLBACK_AFTER_FAILURES).toBe(3);
    expect(connection.GetMode()).toBe('polling');
    expect(seen).toEqual([]);
    vi.advanceTimersByTime(POLL_MS);
    expect(seen).toEqual([{ Kind: 'poll' }]);
    vi.advanceTimersByTime(POLL_MS * 2);
    expect(seen.filter((signal) => signal.Kind === 'poll')).toHaveLength(3);
    connection.Stop();
  });

  it('polls only while the tab is visible', () => {
    const { connection, seen } = make();
    connection.Start();
    for (let i = 0; i < FALLBACK_AFTER_FAILURES; i += 1) {
      FakeEventSource.latest().fail(true);
      vi.advanceTimersByTime(BACKOFF_MS[i] ?? 0);
    }
    expect(connection.GetMode()).toBe('polling');
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    vi.advanceTimersByTime(POLL_MS * 3);
    expect(seen).toEqual([]);
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
    document.dispatchEvent(new Event('visibilitychange'));
    expect(seen).toEqual([{ Kind: 'poll' }]);
    connection.Stop();
  });

  it('tries the stream again every minute while polling and stops polling when it opens', () => {
    const { connection, seen } = make();
    connection.Start();
    for (let i = 0; i < FALLBACK_AFTER_FAILURES; i += 1) {
      FakeEventSource.latest().fail(true);
      vi.advanceTimersByTime(BACKOFF_MS[i] ?? 0);
    }
    const before = FakeEventSource.instances.length;
    vi.advanceTimersByTime(RETRY_STREAM_MS);
    expect(FakeEventSource.instances.length).toBe(before + 1);
    FakeEventSource.latest().open();
    expect(connection.GetMode()).toBe('live');
    expect(seen.at(-1)).toEqual({ Kind: 'reset' });
    const polls = seen.filter((signal) => signal.Kind === 'poll').length;
    vi.advanceTimersByTime(POLL_MS * 4);
    expect(seen.filter((signal) => signal.Kind === 'poll')).toHaveLength(polls);
    connection.Stop();
  });

  it('treats a stream that never opens as a failure, because a proxy may hold it back', () => {
    const { connection } = make();
    connection.Start();
    vi.advanceTimersByTime(OPEN_WATCHDOG_MS);
    expect(FakeEventSource.latest().readyState).toBe(FakeEventSource.CLOSED);
    vi.advanceTimersByTime(BACKOFF_MS[0]);
    expect(FakeEventSource.instances).toHaveLength(2);
    connection.Stop();
  });

  it('goes straight to polling where the browser has no EventSource', () => {
    const connection = new OrgEventsConnection(ORG, () => null);
    const seen: TOrgSignal[] = [];
    connection.Subscribe((signal) => seen.push(signal));
    connection.Start();
    expect(connection.GetMode()).toBe('polling');
    vi.advanceTimersByTime(POLL_MS);
    expect(seen).toEqual([{ Kind: 'poll' }]);
    connection.Stop();
  });

  it('closes everything on stop', () => {
    const { connection, seen } = make();
    connection.Start();
    FakeEventSource.latest().open();
    connection.Stop();
    expect(FakeEventSource.latest().readyState).toBe(FakeEventSource.CLOSED);
    vi.advanceTimersByTime(RETRY_STREAM_MS * 2);
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(seen).toEqual([]);
  });
});

describe('the registry', () => {
  const registry = () => {
    FakeEventSource.instances = [];
    return new OrgEventsRegistry((url) => new FakeEventSource(url) as unknown as EventSource);
  };

  it('shares one stream between every subscriber of an organization', () => {
    const shared = registry();
    const a = shared.Acquire(ORG);
    const b = shared.Acquire(ORG);
    expect(a.Connection).toBe(b.Connection);
    expect(FakeEventSource.instances).toHaveLength(1);
    shared.Acquire('0198a000-0000-7000-8000-0000000000a2');
    expect(FakeEventSource.instances).toHaveLength(2);
  });

  it('closes the stream after the last subscriber leaves, with a short grace for a page change', () => {
    const shared = registry();
    const a = shared.Acquire(ORG);
    a.Release();
    vi.advanceTimersByTime(CLOSE_GRACE_MS - 1);
    const again = shared.Acquire(ORG);
    vi.advanceTimersByTime(CLOSE_GRACE_MS * 2);
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(FakeEventSource.latest().readyState).not.toBe(FakeEventSource.CLOSED);
    again.Release();
    again.Release();
    vi.advanceTimersByTime(CLOSE_GRACE_MS);
    expect(FakeEventSource.latest().readyState).toBe(FakeEventSource.CLOSED);
    expect(shared.Size()).toBe(0);
  });
});

describe('parseOrgEvent', () => {
  it('accepts an event with a type and a sequence and nothing else', () => {
    expect(parseOrgEvent(JSON.stringify(event(3)))).toEqual(event(3));
    for (const bad of [null, 5, '', '[]', '{"Type":"x"}', '{"Seq":1}', 'null']) expect(parseOrgEvent(bad)).toBeNull();
  });
});

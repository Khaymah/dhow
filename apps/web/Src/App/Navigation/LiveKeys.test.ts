import { describe, expect, it } from 'vitest';

import { queryKeysFor } from '@/App/Navigation/LiveKeys';
import type { IOrgEvent } from '@/Core/Events/OrgEvents';

const ORG = 'org-1';
const PROJECT = 'project-1';

const event = (Type: string, ProjectId: string | null = PROJECT): { Kind: 'event'; Event: IOrgEvent } => ({
  Kind: 'event',
  Event: { Seq: 1, Type, ProjectId: ProjectId ?? undefined, At: '2026-11-07T10:00:00Z' },
});

const prefixes = (signalKeys: readonly (readonly unknown[])[]): string[] => signalKeys.map((key) => key.join('/'));

describe('which queries a live signal refreshes', () => {
  it('refreshes the board and the environments of the project for a release or target event, and the overview counters', () => {
    for (const type of ['release.switched', 'release.rollback_failed', 'target.checked']) {
      expect(prefixes(queryKeysFor(ORG, event(type)))).toEqual([`overview/${ORG}`, `boards/${ORG}/${PROJECT}`, `environments/${ORG}/${PROJECT}`]);
    }
  });

  it('refreshes the approvals too for a deployment or an approval', () => {
    for (const type of ['deployment.finished', 'approval.requested', 'approval.decided']) {
      expect(prefixes(queryKeysFor(ORG, event(type)))).toContain(`approvals/${ORG}`);
    }
  });

  it('refreshes only the overview for an event about the counters', () => {
    expect(prefixes(queryKeysFor(ORG, event('overview.changed', null)))).toEqual([`overview/${ORG}`]);
  });

  it('refreshes every board of the organization when the event names no project', () => {
    expect(prefixes(queryKeysFor(ORG, event('release.succeeded', null)))).toEqual([`overview/${ORG}`, `boards/${ORG}`, `environments/${ORG}`]);
  });

  it('refreshes only the inbox for an inbox event', () => {
    expect(prefixes(queryKeysFor(ORG, event('inbox.changed', null)))).toEqual(['inbox']);
  });

  it('refreshes everything and the inbox after a reset, and everything but the inbox after a poll', () => {
    const boards = [`overview/${ORG}`, `approvals/${ORG}`, `boards/${ORG}`, `environments/${ORG}`];
    expect(prefixes(queryKeysFor(ORG, { Kind: 'reset' }))).toEqual([...boards, 'inbox']);
    expect(prefixes(queryKeysFor(ORG, { Kind: 'poll' }))).toEqual(boards);
  });
});

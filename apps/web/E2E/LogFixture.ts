import type { Page } from '@playwright/test';

import type { components } from '../Src/Core/Api/Schema';

type TDetail = components['schemas']['JobDetail'];

export const LOG_JOB_ID = '01980000-0000-7000-8000-000000000a01';
const CHUNK_LINES = 50;
const AT = '2026-11-07T10:00:00Z';

export const lineText = (index: number): string => `perf line ${String(index).padStart(6, '0')} dhow baseline output`;

export const jobDetail = (organizationId: string): TDetail => ({
  Job: {
    Id: LOG_JOB_ID,
    OrganizationId: organizationId,
    Name: 'perf-baseline',
    State: 'succeeded',
    Labels: [],
    Isolation: 'container',
    Priority: 0,
    Image: 'docker.io/library/alpine@sha256:0000000000000000000000000000000000000000000000000000000000000000',
    SpecDigest: '0'.repeat(64),
    AttemptCount: 1,
    MaxInfraRetries: 2,
    QueuedAt: AT,
    StartedAt: AT,
    FinishedAt: AT,
    QueueDeadline: AT,
    CreatedAt: AT,
  },
  Spec: {
    Image: 'docker.io/library/alpine@sha256:0000000000000000000000000000000000000000000000000000000000000000',
    Steps: [{ Name: 'Print', Run: 'true' }],
  },
  Attempts: [
    {
      Id: '01980000-0000-7000-8000-000000000a02',
      Number: 1,
      RunnerId: '01980000-0000-7000-8000-000000000a03',
      FencingToken: 1,
      State: 'succeeded',
      LeasedAt: AT,
      StartedAt: AT,
      FinishedAt: AT,
      Steps: [{ Index: 0, Name: 'Print', Mutating: false, State: 'succeeded', StartedAt: AT, FinishedAt: AT }],
    },
  ],
});

export const logStream = (lines: number): string => {
  const events: string[] = [];
  for (let first = 1; first <= lines; first += CHUNK_LINES) {
    const last = Math.min(lines, first + CHUNK_LINES - 1);
    const text = Array.from({ length: last - first + 1 }, (_, offset) => lineText(first + offset)).join('\n') + '\n';
    const chunk = { Seq: first, Stream: 'stdout', StepIndex: 0, CreatedAt: AT, Text: text };
    events.push(`event: chunk\ndata: ${JSON.stringify(chunk)}\n\n`);
  }
  events.push(`event: end\ndata: ${JSON.stringify({ Attempt: 1, State: 'succeeded', Truncated: false })}\n\n`);
  return events.join('');
};

export const stubLog = async (page: Page, organizationId: string, lines: number): Promise<void> => {
  await page.route(`**/api/v1/organizations/${organizationId}/jobs/${LOG_JOB_ID}`, (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ Success: true, Data: jobDetail(organizationId) }) }),
  );
  await page.route(`**/api/v1/organizations/${organizationId}/jobs/${LOG_JOB_ID}/logs/stream**`, (route) =>
    route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store' },
      body: logStream(lines),
    }),
  );
};

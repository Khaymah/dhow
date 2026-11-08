import type { Page } from '@playwright/test';

import type { components } from '../Src/Core/Api/Schema';

type TBoard = components['schemas']['DeployBoard'];
type TEnvironment = components['schemas']['DeployBoardEnvironment'];
type TTarget = components['schemas']['DeployBoardTarget'];

const AT = '2026-11-07T10:00:00Z';
const ID_PREFIX = '01980000-0000-7000-8000-';

export const BOARD_ENVIRONMENTS = 200;
export const BOARD_TARGETS_PER_ENVIRONMENT = 2;

const idOf = (kind: number, index: number): string => `${ID_PREFIX}${String(kind)}${String(index).padStart(11, '0')}`;

export const environmentName = (index: number): string => `perf-env-${String(index).padStart(3, '0')}`;

const targetOf = (environment: number, index: number): TTarget => {
  const running = (environment + index) % 17 === 0;
  const failed = (environment + index) % 23 === 0;
  return {
    Id: idOf(2, environment * 10 + index),
    Name: `app-${String(index)}`,
    Kind: 'ssh',
    Hosts: 6,
    CurrentReleaseId: `rel-${String(environment)}`,
    CurrentDigest: String(environment).padStart(4, '0').repeat(16),
    Last: {
      Id: idOf(3, environment * 10 + index),
      ReleaseId: `rel-${String(environment)}`,
      Kind: 'deploy',
      State: failed ? 'rollback_failed' : 'succeeded',
      StartedAt: AT,
      FinishedAt: AT,
    },
    Running: running ? { ReleaseId: `rel-${String(environment + 1)}`, Hosts: 6, HostsDone: 2, BatchSize: 2, Batch: 2, Batches: 3 } : undefined,
    Partial: failed,
    AutoRolledBack: false,
    RollbackFailed: failed,
  };
};

export const boardOf = (environments: number, targetsPer: number): TBoard => ({
  Environments: Array.from({ length: environments }, (_, environment): TEnvironment => ({
    Id: idOf(1, environment),
    Name: environmentName(environment),
    Paused: environment % 31 === 0,
    PendingApprovals: environment % 13 === 0 ? 1 : 0,
    Targets: Array.from({ length: targetsPer }, (__, index) => targetOf(environment, index)),
  })),
  Truncated: false,
});

export const stubBoard = async (page: Page, organizationId: string, board: TBoard): Promise<void> => {
  const body = JSON.stringify({ Success: true, Message: 'boards.deploy.loaded', Data: board });
  await page.route(new RegExp(`/api/v1/organizations/${organizationId}/projects/[^/]+/deploy-board$`), async (route) => {
    if (route.request().headers()['if-none-match'] === '"perf"') {
      await route.fulfill({ status: 304, body: '' });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', headers: { ETag: '"perf"' }, body });
  });
};

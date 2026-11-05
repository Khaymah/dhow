import { createMemoryHistory } from '@tanstack/react-router';
import { render } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { approvalsRoute, gatedJob, newApprovalsState, type IApprovalsState } from '@/Core/Testing/ApprovalsBackend';
import { startBackend } from '@/Core/Testing/Backend';
import { environmentFixture, environmentsRoute, newEnvironmentsState, rulesFixture } from '@/Core/Testing/EnvironmentsBackend';
import { detailFixture, ok, PROJECT_ID, projectFixture, projectsRoute, RUN_ID, runFixture, type TFixture } from '@/Core/Testing/ProjectsBackend';

export const RUN_URL = `/projects/${PROJECT_ID}/runs/${RUN_ID}`;

export interface IRunMountOptions {
  role?: string;
  jobs?: TFixture[];
  state?: IApprovalsState;
  bypass?: boolean;
  secondFactor?: boolean;
  url?: string;
}

export const mountRun = (options: IRunMountOptions = {}) => {
  const role = options.role ?? 'owner';
  const state = options.state ?? newApprovalsState();
  const detail = { current: detailFixture(runFixture({ State: 'running', FinishedAt: undefined }), options.jobs ?? [gatedJob('approval')]) };
  const reads = { runs: 0 };
  const environments = newEnvironmentsState();
  environments.environments = [
    environmentFixture({ Rules: rulesFixture({ AllowAdminBypass: options.bypass ?? true }) }),
    environmentFixture({ Name: 'staging', Id: '0198a000-0000-7000-8000-000000000a02', Rules: rulesFixture({ AllowAdminBypass: true }) }),
  ];
  const backend = startBackend({
    role,
    mfa: options.secondFactor === false ? undefined : { TotpEnabled: true, RecoveryCodesRemaining: 4, Passkeys: [] },
    routes: [
      approvalsRoute(state),
      environmentsRoute(environments),
      projectsRoute({
        project: () => ok(projectFixture({ CallerRole: role })),
        run: () => {
          reads.runs += 1;
          return ok(detail.current);
        },
      }),
    ],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [options.url ?? RUN_URL] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, state, detail, reads, environments };
};

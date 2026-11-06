import { createMemoryHistory } from '@tanstack/react-router';
import { render } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { App } from '@/App/App';
import { createAppRouter } from '@/App/Router';
import { createQueryClient } from '@/Core/Api/QueryClient';
import { startBackend } from '@/Core/Testing/Backend';
import { ORG_ID } from '@/Core/Testing/Fetch';
import { environmentFixture, environmentsRoute, ENVIRONMENT_NAME, newEnvironmentsState } from '@/Core/Testing/EnvironmentsBackend';
import { ok, PROJECT_ID, projectFixture, projectsRoute } from '@/Core/Testing/ProjectsBackend';
import { newTargetsState, targetsRoute, type ITargetsState } from '@/Core/Testing/TargetsBackend';
import { newTargetsManageState, targetsManageRoute, type ITargetsManageState } from '@/Core/Testing/TargetsManageBackend';

export const PASSWORD = 'correct horse battery staple';
export const ENVIRONMENT_URL = `/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`;
export const TARGET_URL = `${ENVIRONMENT_URL}/targets/web`;
export const NEW_TARGET_URL = `${ENVIRONMENT_URL}/new-target`;
export const API = `/organizations/${ORG_ID}/projects/${PROJECT_ID}/environments/${ENVIRONMENT_NAME}`;

export interface IMountOptions {
  url: string;
  role?: string;
  manage?: ITargetsManageState;
  targets?: ITargetsState;
  paused?: boolean;
  organizationRole?: string;
}

export const mountTargets = ({ url, role = 'owner', manage, targets, paused = false, organizationRole = role }: IMountOptions) => {
  const manageState = manage ?? newTargetsManageState();
  const targetsState = targets ?? newTargetsState();
  const environments = newEnvironmentsState();
  environments.environments = [paused ? environmentFixture({ Paused: true, PausedReason: 'Freeze' }) : environmentFixture()];
  const backend = startBackend({
    role: organizationRole,
    routes: [
      targetsManageRoute(manageState),
      targetsRoute(targetsState),
      environmentsRoute(environments),
      projectsRoute({ project: () => ok(projectFixture({ CallerRole: role })) }),
    ],
  });
  const router = createAppRouter(createMemoryHistory({ initialEntries: [url] }));
  const client = createQueryClient({ retry: false });
  const user = userEvent.setup();
  render(<App router={router} queryClient={client} />);
  return { ...backend, router, client, user, manage: manageState, targets: targetsState };
};

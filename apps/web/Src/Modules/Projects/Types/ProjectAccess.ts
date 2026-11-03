import type { TRole } from '@/Core/Auth/AuthTypes';

export type TProjectAction = 'project.manage' | 'run.trigger' | 'run.cancel' | 'run.approve' | 'artifact.read';

const MANAGERS: readonly TRole[] = ['owner', 'admin', 'maintainer'];
const TRIGGERS: readonly TRole[] = ['owner', 'admin', 'maintainer', 'developer'];
const FORK_ADMINS: readonly TRole[] = ['owner', 'admin'];

const GRANTS: Record<TProjectAction, readonly TRole[]> = {
  'project.manage': MANAGERS,
  'run.trigger': TRIGGERS,
  'run.cancel': MANAGERS,
  'run.approve': MANAGERS,
  'artifact.read': TRIGGERS,
};

export const canOnProject = (role: TRole | undefined, action: TProjectAction): boolean =>
  role !== undefined && GRANTS[action].includes(role);

export { canCreateProjects } from '@/Core/Auth/AuthTypes';

export const canRunForksWithoutApproval = (role: TRole | undefined): boolean => role !== undefined && FORK_ADMINS.includes(role);

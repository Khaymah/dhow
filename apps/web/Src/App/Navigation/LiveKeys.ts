import type { QueryKey } from '@tanstack/react-query';

import type { IOrgEvent, TOrgSignal } from '@/Core/Events/OrgEvents';
import { approvalsKey } from '@/Modules/Approvals/Api/ApprovalsApi';
import { boardKey, boardsKey } from '@/Modules/Boards/Api/BoardsApi';
import { environmentsKey } from '@/Modules/Environments/Api/EnvironmentsApi';
import { INBOX_KEY } from '@/Modules/Notifications/Api/InboxKey';
import { overviewKey } from '@/Modules/Overview/Hooks/useOverview';

const everything = (organizationId: string): QueryKey[] => [
  overviewKey(organizationId),
  approvalsKey(organizationId),
  boardsKey(organizationId),
  ['environments', organizationId],
];

const projectKeys = (organizationId: string, projectId: string | undefined): QueryKey[] =>
  projectId === undefined
    ? [boardsKey(organizationId), ['environments', organizationId]]
    : [boardKey(organizationId, projectId), environmentsKey(organizationId, projectId)];

const keysOfEvent = (organizationId: string, event: IOrgEvent): QueryKey[] => {
  const overview = overviewKey(organizationId);
  const group = event.Type.split('.')[0];
  switch (group) {
    case 'approval':
    case 'deployment':
      return [overview, approvalsKey(organizationId), ...projectKeys(organizationId, event.ProjectId)];
    case 'inbox':
      return [INBOX_KEY];
    case 'release':
    case 'target':
      return [overview, ...projectKeys(organizationId, event.ProjectId)];
    default:
      return [overview];
  }
};

export const queryKeysFor = (organizationId: string, signal: TOrgSignal): QueryKey[] =>
  signal.Kind === 'event'
    ? keysOfEvent(organizationId, signal.Event)
    : signal.Kind === 'auth'
      ? []
      : signal.Kind === 'reset'
        ? [...everything(organizationId), INBOX_KEY]
        : everything(organizationId);

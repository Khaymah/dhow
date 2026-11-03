import type { ReactNode } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import type { TRole } from '@/Core/Auth/AuthTypes';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useCrumbTitle } from '@/Core/Hooks/useCrumbTitle';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TProject } from '@/Modules/Projects/Api/ProjectsApi';
import { useOrganizationAccess } from '@/Modules/Projects/Hooks/useProjectAccess';
import { useProject } from '@/Modules/Projects/Hooks/useProjects';

export interface IProjectView {
  organizationId: string;
  project: TProject;
  role: TRole;
}

export interface IProjectGateProps {
  projectId: string;
  fallback?: (body: ReactNode) => ReactNode;
  children: (view: IProjectView) => ReactNode;
}

const same = (body: ReactNode): ReactNode => body;

const Loaded = ({ organizationId, projectId, fallbackRole, fallback = same, children }: IProjectGateProps & { organizationId: string; fallbackRole: TRole }) => {
  const t = useT();
  const errorText = useApiErrorText();
  const project = useProject(organizationId, projectId);
  useCrumbTitle(projectId, project.data?.Name);

  if (isApiError(project.error) && project.error.Status === 404) {
    return fallback(
      <Card>
        <EmptyState icon="folder" text={t('projects.detail.notFound')} />
      </Card>,
    );
  }
  if (project.isError) {
    return fallback(
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('projects.detail.error')}>
          {errorText(project.error)}
        </InlineAlert>
        <Button size="sm" onClick={() => void project.refetch()}>
          {t('projects.retry')}
        </Button>
      </div>,
    );
  }
  if (project.data === undefined) return fallback(<Skeleton label={t('projects.detail.loading')} lines={6} />);
  return <>{children({ organizationId, project: project.data, role: project.data.CallerRole ?? fallbackRole })}</>;
};

export const ProjectGate = ({ projectId, fallback = same, children }: IProjectGateProps) => {
  const t = useT();
  const access = useOrganizationAccess();
  if (access.Kind === 'loading') return fallback(<Skeleton label={t('projects.detail.loading')} lines={6} />);
  if (access.Kind === 'denied') {
    return fallback(
      <Card>
        <EmptyState icon="shield" text={t('projects.forbidden')} />
      </Card>,
    );
  }
  return (
    <Loaded organizationId={access.OrganizationId} projectId={projectId} fallbackRole={access.Role} fallback={fallback}>
      {children}
    </Loaded>
  );
};

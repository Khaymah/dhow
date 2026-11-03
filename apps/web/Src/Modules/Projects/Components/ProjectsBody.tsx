import { useState } from 'react';

import { useCurrentMembership } from '@/Core/Auth/useMe';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Pager } from '@/Core/Ui/Pager';
import { Rows } from '@/Core/Ui/Row';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { PROJECTS_PAGE_SIZE, type TProjectState } from '@/Modules/Projects/Api/ProjectsApi';
import { FilterChips } from '@/Modules/Projects/Components/FilterChips';
import { LinkRow } from '@/Modules/Projects/Components/LinkRow';
import { useOrganizationAccess } from '@/Modules/Projects/Hooks/useProjectAccess';
import { useProjects } from '@/Modules/Projects/Hooks/useProjects';
import { canCreateProjects } from '@/Modules/Projects/Types/ProjectAccess';

export interface IProjectsBodyProps {
  hrefFor: (projectId: string) => string;
  onOpenProject: (projectId: string) => void;
  onCreate: () => void;
}

interface IListProps extends IProjectsBodyProps {
  organizationId: string;
  creator: boolean;
}

const STATES: readonly TProjectState[] = ['active', 'archived'];

const ProjectList = ({ organizationId, creator, hrefFor, onOpenProject, onCreate }: IListProps) => {
  const t = useT();
  const time = useRelativeTime();
  const errorText = useApiErrorText();
  const [State, setState] = useState<TProjectState>('active');
  const [Page, setPage] = useState(1);
  const projects = useProjects(organizationId, Page, State);

  const tabs = (
    <FilterChips
      label={t('projects.filter.label')}
      chips={STATES.map((state) => ({ id: state, label: t(`projects.filter.${state}`) }))}
      value={State}
      onChange={(next) => {
        setState(next);
        setPage(1);
      }}
    />
  );

  if (projects.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        {tabs}
        <InlineAlert accent="failed" title={t('projects.error')}>
          {errorText(projects.error)}
        </InlineAlert>
        <Button size="sm" onClick={() => void projects.refetch()}>
          {t('projects.retry')}
        </Button>
      </div>
    );
  }
  if (projects.data === undefined) return <Skeleton label={t('projects.loading')} lines={5} />;

  const { Items, Meta } = projects.data;
  return (
    <>
      {tabs}
      {Items.length === 0 ? (
        <Card padded>
          <EmptyState
            icon="folder"
            text={State === 'archived' ? t('projects.empty.archived') : creator ? t('projects.empty.creator') : t('projects.empty.member')}
            action={
              creator && State === 'active' ? (
                <Button iconStart="plus" onClick={onCreate}>
                  {t('projects.create')}
                </Button>
              ) : null
            }
          />
        </Card>
      ) : (
        <Card title={t('projects.list.title')} icon="folder" end={t('projects.list.total', { count: Meta.Total })}>
          <Rows label={t('projects.list.title')}>
            {Items.map((project) => (
              <LinkRow
                key={project.Id}
                icon="folder"
                href={hrefFor(project.Id)}
                title={<bdi>{project.Name}</bdi>}
                sub={
                  <>
                    <bdi dir="ltr">{project.Slug}</bdi>
                    {` · ${t('projects.row.created', { time: time.ago(project.CreatedAt) })}`}
                  </>
                }
                end={
                  project.State === 'archived' ? (
                    <Badge tone="mute" icon="queue">
                      {t('projects.state.archived')}
                    </Badge>
                  ) : undefined
                }
                onOpen={() => {
                  onOpenProject(project.Id);
                }}
              />
            ))}
          </Rows>
        </Card>
      )}
      <Pager page={Meta.Page} limit={Meta.Limit || PROJECTS_PAGE_SIZE} total={Meta.Total} busy={projects.isFetching} onPage={setPage} />
    </>
  );
};

export const ProjectsBody = (props: IProjectsBodyProps) => {
  const t = useT();
  const access = useOrganizationAccess();
  if (access.Kind === 'loading') return <Skeleton label={t('projects.loading')} lines={5} />;
  if (access.Kind === 'denied') {
    return (
      <Card>
        <EmptyState icon="shield" text={t('projects.forbidden')} />
      </Card>
    );
  }
  return <ProjectList {...props} organizationId={access.OrganizationId} creator={canCreateProjects(access.Role)} />;
};

export const ProjectsFooter = ({ onCreate }: Pick<IProjectsBodyProps, 'onCreate'>) => {
  const t = useT();
  const role = useCurrentMembership()?.Role;
  if (!canCreateProjects(role)) return null;
  return (
    <>
      <span className="flex-1" />
      <Button variant="primary" iconStart="plus" onClick={onCreate}>
        {t('projects.create')}
      </Button>
    </>
  );
};

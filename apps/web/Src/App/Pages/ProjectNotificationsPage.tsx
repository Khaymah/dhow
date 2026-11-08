import { useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { PageGroup } from '@/App/Layout/PageGroup';
import { useSlide } from '@/App/Navigation/useSlide';
import { isUuid } from '@/App/Pages/Params';
import { ProjectNav } from '@/App/Pages/ProjectNav';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { ChannelsCard } from '@/Modules/Notifications/Components/ChannelsCard';
import { DeliveriesCard } from '@/Modules/Notifications/Components/DeliveriesCard';
import { canManageNotifications } from '@/Modules/Notifications/Types/NotificationsAccess';
import { ProjectGate } from '@/Modules/Projects/Components/ProjectGate';

export const ProjectNotificationsPage = () => {
  const t = useT();
  const { open } = useSlide();
  const { projectId } = useParams({ strict: false });
  if (!isUuid(projectId)) return <NotFoundPage />;

  return (
    <ProjectGate projectId={projectId} fallback={(body) => <Page title={t('projects.nav.notifications')}>{body}</Page>}>
      {({ organizationId, project, role }) => {
        const manage = canManageNotifications(role);
        const active = project.State === 'active';
        return (
          <Page title={project.Name} lead={t('notifications.project.lead')} nav={<ProjectNav projectId={projectId} current="notifications" role={role} />}>
            {active ? null : <InlineAlert accent="building" title={t('projects.archived.blocked')} />}
            {manage ? null : <InlineAlert accent="building" title={t('notifications.readOnly')} />}
            <PageGroup title={t('notifications.channels.group')} icon="bell">
              <ChannelsCard
                organizationId={organizationId}
                projectId={projectId}
                manage={manage}
                writable={active}
                onNew={() => {
                  open('notification-channel-new');
                }}
              />
            </PageGroup>
            {manage ? (
              <PageGroup title={t('notifications.deliveries.group')} icon="file">
                <DeliveriesCard organizationId={organizationId} projectId={projectId} />
              </PageGroup>
            ) : null}
          </Page>
        );
      }}
    </ProjectGate>
  );
};

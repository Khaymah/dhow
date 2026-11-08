import { useParams } from '@tanstack/react-router';

import { NotFoundPage } from '@/App/Layout/NotFoundPage';
import { Page } from '@/App/Layout/Page';
import { SettingsGroup } from '@/App/Layout/SettingsGroup';
import { SettingsToc } from '@/App/Layout/SettingsToc';
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
            <SettingsToc page="projectNotifications" context={{ role, owner: false, projectActive: active }} />
            {active ? null : <InlineAlert accent="building" title={t('projects.archived.blocked')} />}
            {manage ? null : <InlineAlert accent="building" title={t('notifications.readOnly')} />}
            <SettingsGroup setting="project.channels">
              <ChannelsCard
                organizationId={organizationId}
                projectId={projectId}
                manage={manage}
                writable={active}
                onNew={() => {
                  open('notification-channel-new');
                }}
              />
            </SettingsGroup>
            {manage ? (
              <SettingsGroup setting="project.deliveries">
                <DeliveriesCard organizationId={organizationId} projectId={projectId} />
              </SettingsGroup>
            ) : null}
          </Page>
        );
      }}
    </ProjectGate>
  );
};

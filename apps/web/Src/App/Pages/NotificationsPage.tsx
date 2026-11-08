import { Page } from '@/App/Layout/Page';
import { useT } from '@/I18n/useT';
import { InboxList } from '@/Modules/Notifications/Components/InboxList';

export const NotificationsPage = () => {
  const t = useT();

  return (
    <Page title={t('notifications.title')} lead={t('page.notifications.lead')}>
      <InboxList />
    </Page>
  );
};

import { useNavigate } from '@tanstack/react-router';

import { Page } from '@/App/Layout/Page';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { MyApprovalsCard } from '@/Modules/Approvals/Components/MyApprovalsCard';

export const ApprovalsPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const membership = useCurrentMembership();

  return (
    <Page title={t('approvals.page.title')} lead={t('approvals.page.lead')}>
      {membership === undefined ? (
        <Skeleton label={t('ui.loading')} lines={3} />
      ) : (
        <MyApprovalsCard
          organizationId={membership.OrganizationId}
          hrefFor={(approval) => `/projects/${approval.ProjectId}/runs/${approval.RunId}`}
          onOpen={(approval) => {
            void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId: approval.ProjectId, runId: approval.RunId } });
          }}
        />
      )}
    </Page>
  );
};

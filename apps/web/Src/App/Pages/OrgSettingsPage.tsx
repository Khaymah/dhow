import { Page } from '@/App/Layout/Page';
import { PageGroup } from '@/App/Layout/PageGroup';
import { OrgNav } from '@/App/Layout/SectionNavs';
import { useSlide } from '@/App/Navigation/useSlide';
import { OrganizationGate } from '@/App/Pages/OrganizationGate';
import { canEditOrganization } from '@/Core/Auth/AuthTypes';
import { useT } from '@/I18n/useT';
import { OrganizationCard } from '@/Modules/Organization/Components/OrganizationCard';
import { SecondFactorCard } from '@/Modules/Organization/Components/SecondFactorCard';
import { LogRetentionCard, LogRetentionSummary } from '@/Modules/Organization/Components/LogRetentionCard';
import { DangerZone, SecondFactorSummary } from '@/Modules/Organization/Components/SettingsSection';

export const OrgSettingsPage = () => {
  const t = useT();
  const { open } = useSlide();

  return (
    <Page title={t('shell.nav.orgSettings')} lead={t('page.orgSettings.lead')} nav={<OrgNav />}>
      <OrganizationGate>
        {(organization, membership) => (
          <>
            <PageGroup title={t('orgSettings.group.general')} icon="building">
              <OrganizationCard organization={organization} editable={canEditOrganization(membership.Role)} />
            </PageGroup>
            <PageGroup title={t('orgSettings.group.security')} icon="shield">
              {membership.Role === 'owner' ? (
                <SecondFactorCard organization={organization} />
              ) : (
                <SecondFactorSummary organization={organization} />
              )}
              {membership.Role === 'owner' ? (
                <LogRetentionCard organization={organization} />
              ) : (
                <LogRetentionSummary organization={organization} />
              )}
            </PageGroup>
            <PageGroup title={t('orgSettings.group.danger')} lead={t('orgSettings.danger.lead')} icon="alert" danger>
              <DangerZone
                organization={organization}
                role={membership.Role}
                onTransfer={() => {
                  open('organization-transfer');
                }}
                onLeave={() => {
                  open('organization-leave');
                }}
              />
            </PageGroup>
          </>
        )}
      </OrganizationGate>
    </Page>
  );
};

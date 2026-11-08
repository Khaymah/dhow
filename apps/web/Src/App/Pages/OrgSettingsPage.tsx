import { Page } from '@/App/Layout/Page';
import { OrgNav } from '@/App/Layout/SectionNavs';
import { SettingsGroup } from '@/App/Layout/SettingsGroup';
import { SettingsToc } from '@/App/Layout/SettingsToc';
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
            <SettingsToc page="org" context={{ role: membership.Role, owner: false, projectActive: true }} />
            <SettingsGroup setting="org.general">
              <OrganizationCard organization={organization} editable={canEditOrganization(membership.Role)} />
            </SettingsGroup>
            <SettingsGroup setting="org.security">
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
            </SettingsGroup>
            <SettingsGroup setting="org.danger" lead={t('orgSettings.danger.lead')} danger>
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
            </SettingsGroup>
          </>
        )}
      </OrganizationGate>
    </Page>
  );
};

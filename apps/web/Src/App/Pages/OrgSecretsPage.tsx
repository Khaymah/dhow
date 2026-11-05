import { Page } from '@/App/Layout/Page';
import { OrgNav } from '@/App/Layout/SectionNavs';
import { RegistryGroup, SecretsGroups } from '@/App/Pages/SecretsGroups';
import { canManageOrganizationSecrets } from '@/Core/Auth/AuthTypes';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';

export const OrgSecretsPage = () => {
  const t = useT();
  const membership = useCurrentMembership();
  const writable = canManageOrganizationSecrets(membership?.Role);

  return (
    <Page title={t('shell.nav.secrets')} lead={t('secrets.org.lead')} nav={<OrgNav />}>
      {membership === undefined ? (
        <Skeleton label={t('ui.loading')} lines={5} />
      ) : (
        <>
          {writable ? null : <InlineAlert accent="building" title={t('secrets.readOnly')} />}
          <SecretsGroups
            key={membership.OrganizationId}
            scope={{
              OrganizationId: membership.OrganizationId,
              ProjectId: null,
            }}
            writable={writable}
          />
          <RegistryGroup
            key={`${membership.OrganizationId}-registry`}
            scope={{
              OrganizationId: membership.OrganizationId,
              ProjectId: null,
            }}
            writable={writable}
          />
        </>
      )}
    </Page>
  );
};

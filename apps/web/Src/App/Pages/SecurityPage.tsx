import { Page } from '@/App/Layout/Page';
import { AccountNav } from '@/App/Layout/SectionNavs';
import { SettingsGroup } from '@/App/Layout/SettingsGroup';
import { SettingsToc } from '@/App/Layout/SettingsToc';
import { useSlide } from '@/App/Navigation/useSlide';
import { useMe } from '@/Core/Auth/useMe';
import { useMfaStatus } from '@/Core/Auth/useMfaStatus';
import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';
import { MfaSection } from '@/Modules/Mfa/Components/MfaSection';
import { PasswordCard, SecuritySummary } from '@/Modules/Session/Components/AccountBody';
import { IdentitiesCard } from '@/Modules/Session/Components/IdentitiesCard';
import { SessionsCard } from '@/Modules/Session/Components/SessionsCard';
import { TokensCard } from '@/Modules/Tokens/Components/TokensCard';

export const SecurityPage = () => {
  const t = useT();
  const me = useMe();
  const status = useMfaStatus();
  const { open } = useSlide();
  const HandleTotp = () => {
    open('account-totp');
  };

  return (
    <Page
      title={t('shell.nav.security')}
      lead={t('page.security.lead')}
      action={
        status.data !== undefined && !status.data.Enabled ? (
          <Button variant="primary" iconStart="shield" onClick={HandleTotp}>
            {t('security.enable')}
          </Button>
        ) : undefined
      }
      nav={
        <>
          <AccountNav />
          <SettingsToc page="security" context={{ role: undefined, owner: true, projectActive: true }} />
        </>
      }
    >
      {me.data ? <SecuritySummary me={me.data} /> : null}
      <SettingsGroup setting="security.password">
        <PasswordCard
          onChange={() => {
            open('account-password');
          }}
        />
        <IdentitiesCard />
      </SettingsGroup>
      <SettingsGroup setting="security.secondFactor">
        <MfaSection onSetUpTotp={HandleTotp} />
      </SettingsGroup>
      <SettingsGroup setting="security.sessions">
        <SessionsCard />
      </SettingsGroup>
      <SettingsGroup setting="security.tokens">
        <TokensCard
          onNew={() => {
            open('account-token-new');
          }}
        />
      </SettingsGroup>
    </Page>
  );
};

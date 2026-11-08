import { Page } from '@/App/Layout/Page';
import { AccountNav } from '@/App/Layout/SectionNavs';
import { SettingsGroup } from '@/App/Layout/SettingsGroup';
import { SettingsToc } from '@/App/Layout/SettingsToc';
import { useSlide } from '@/App/Navigation/useSlide';
import { themeEditSlide } from '@/App/Panels';
import { Card } from '@/Core/Ui/Card';
import { ChoiceList } from '@/Core/Ui/ChoiceList';
import { LOCALES } from '@/I18n/Locale';
import { useI18n } from '@/I18n/useI18n';
import { AppearanceBody } from '@/Modules/Appearance/Components/AppearanceBody';
import { AppearanceFooter } from '@/Modules/Appearance/Components/AppearanceFooter';
import { MuteCard } from '@/Modules/Notifications/Components/MuteCard';

const LanguageCard = () => {
  const { Locale, SetLocale, t } = useI18n();

  return (
    <Card title={t('preferences.language.title')} icon="monitor" padded>
      <div className="grid gap-3">
        <p className="m-0 text-body-sm text-secondary">{t('preferences.language.lead')}</p>
        <ChoiceList
          label={t('preferences.language.title')}
          choices={LOCALES.map((locale) => ({ value: locale, title: t(`language.${locale}`) }))}
          value={Locale}
          onChange={SetLocale}
        />
      </div>
    </Card>
  );
};

export const PreferencesPage = () => {
  const { t } = useI18n();
  const { open } = useSlide();

  return (
    <Page
      title={t('shell.nav.preferences')}
      lead={t('page.preferences.lead')}
      action={
        <AppearanceFooter
          onCreate={() => {
            open('theme-edit');
          }}
        />
      }
      nav={
        <>
          <AccountNav />
          <SettingsToc page="preferences" context={{ role: undefined, owner: true, projectActive: true }} />
        </>
      }
    >
      <SettingsGroup setting="preferences.language">
        <LanguageCard />
      </SettingsGroup>
      <SettingsGroup setting="preferences.notifications">
        <MuteCard />
      </SettingsGroup>
      <SettingsGroup setting="preferences.appearance">
        <AppearanceBody
          onEdit={(themeId) => {
            open(themeEditSlide(themeId));
          }}
        />
      </SettingsGroup>
    </Page>
  );
};

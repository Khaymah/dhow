import { AnchorTarget } from '@/App/Layout/AnchorTarget';
import { Page } from '@/App/Layout/Page';
import { SettingsToc } from '@/App/Layout/SettingsToc';
import { useSlide } from '@/App/Navigation/useSlide';
import { oidcEditSlide } from '@/App/Panels';
import { useT } from '@/I18n/useT';
import { SettingsBody } from '@/Modules/Settings/Components/SettingsBody';

export const SystemSettingsPage = () => {
  const t = useT();
  const { open } = useSlide();

  return (
    <Page
      title={t('shell.nav.systemSettings')}
      lead={t('settings.lead')}
      nav={<SettingsToc page="system" context={{ role: undefined, owner: true, projectActive: true }} />}
    >
      <SettingsBody
        section={(anchor, node) => <AnchorTarget anchor={anchor}>{node}</AnchorTarget>}
        onAddProvider={() => {
          open('oidc-new');
        }}
        onEditProvider={(id) => {
          open(oidcEditSlide(id));
        }}
      />
    </Page>
  );
};

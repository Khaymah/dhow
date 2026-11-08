import { Page } from '@/App/Layout/Page';
import { useT } from '@/I18n/useT';
import { AboutBody } from '@/Modules/System/Components/AboutBody';

export const AboutPage = () => {
  const t = useT();

  return (
    <Page title={t('shell.nav.about')} lead={t('about.lead')}>
      <AboutBody />
    </Page>
  );
};

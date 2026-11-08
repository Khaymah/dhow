import { Page } from '@/App/Layout/Page';
import { HelpPagesCard } from '@/App/Pages/HelpPagesCard';
import { useT } from '@/I18n/useT';
import { HelpContentView } from '@/Modules/Help/Components/HelpContentView';

export const HelpPage = () => {
  const t = useT();
  return (
    <Page title={t('help.title')} lead={t('help.lead')}>
      <HelpContentView />
      <HelpPagesCard />
      <p className="m-0 text-body-sm text-tertiary">{t('help.offline')}</p>
    </Page>
  );
};

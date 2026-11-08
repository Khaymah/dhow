import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useI18n } from '@/I18n/useI18n';
import { HelpPanel } from '@/Modules/Help/Components/HelpPanel';
import { useHelpContent } from '@/Modules/Help/Hooks/useHelpContent';

export const HelpContentView = () => {
  const { t } = useI18n();
  const content = useHelpContent();

  if (content.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('help.error.title')} focus>
          {t('help.error.body')}
        </InlineAlert>
        <Button
          onClick={() => {
            void content.refetch();
          }}
        >
          {t('help.error.retry')}
        </Button>
      </div>
    );
  }
  if (content.data === undefined) return <Skeleton label={t('help.loading')} lines={6} />;
  return <HelpPanel content={content.data} />;
};

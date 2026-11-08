import { Banner } from '@/Core/Ui/Banner';
import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';
import { ReloadBanner } from '@/Modules/System/Components/ReloadBanner';
import { useMeta } from '@/Modules/System/Hooks/useMeta';

const StatusBanner = () => {
  const t = useT();
  const meta = useMeta();

  if (meta.isError) {
    return (
      <Banner
        accent="failed"
        title={t('apiDown.title')}
        body={t('apiDown.body')}
        className="mb-4"
        action={
          <Button
            size="sm"
            disabled={meta.isFetching}
            onClick={() => {
              void meta.refetch();
            }}
          >
            {t('apiDown.retry')}
          </Button>
        }
      />
    );
  }

  if (meta.data?.SetupRequired) {
    return <Banner accent="awaiting" title={t('setup.banner.title')} body={t('setup.banner.body')} className="mb-4" />;
  }

  return null;
};

export const SystemBanners = () => (
  <>
    <ReloadBanner />
    <StatusBanner />
  </>
);

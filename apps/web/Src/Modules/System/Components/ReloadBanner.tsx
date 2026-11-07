import { Banner } from '@/Core/Ui/Banner';
import { Button } from '@/Core/Ui/Button';
import { useT } from '@/I18n/useT';
import { useServerBuildChanged } from '@/Modules/System/Hooks/useServerBuildChanged';

export const ReloadBanner = () => {
  const t = useT();
  const changed = useServerBuildChanged();
  const HandleReload = () => {
    window.location.reload();
  };

  if (!changed) return null;

  return (
    <Banner
      accent="awaiting"
      title={t('about.reload.title')}
      body={t('about.reload.body')}
      className="mb-4"
      action={
        <Button size="sm" variant="primary" onClick={HandleReload}>
          {t('about.reload.action')}
        </Button>
      }
    />
  );
};

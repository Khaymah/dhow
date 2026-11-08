import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useDelayedFlag } from '@/Core/Hooks/useDelayedFlag';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { AboutLinksCard } from '@/Modules/System/Components/AboutLinksCard';
import { AboutVersionCard } from '@/Modules/System/Components/AboutVersionCard';
import { DoctorChecksCard } from '@/Modules/System/Components/DoctorChecksCard';
import { useDoctor } from '@/Modules/System/Hooks/useDoctor';

const SKELETON_DELAY_MS = 300;

export const AboutBody = () => {
  const t = useT();
  const errorText = useApiErrorText();
  const doctor = useDoctor();
  const showSkeleton = useDelayedFlag(doctor.isPending, SKELETON_DELAY_MS);

  return (
    <>
      {showSkeleton ? <Skeleton label={t('about.loading')} lines={4} /> : null}
      {doctor.error ? (
        <div className="grid justify-items-start gap-3">
          <InlineAlert accent="failed" title={t('about.error.title')}>
            {errorText(doctor.error)}
          </InlineAlert>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              void doctor.refetch();
            }}
          >
            {t('status.retry')}
          </Button>
        </div>
      ) : null}
      {doctor.data ? (
        <AboutVersionCard build={doctor.data.ServerBuild} healthy={doctor.data.Health.Status === 'ok'} />
      ) : null}
      {doctor.data?.Checks ? <DoctorChecksCard checks={doctor.data.Checks} /> : null}
      <AboutLinksCard />
      <Card title={t('about.license.title')} icon="shield" padded>
        <p className="m-0 text-body-sm text-secondary">{t('about.license.body')}</p>
      </Card>
    </>
  );
};

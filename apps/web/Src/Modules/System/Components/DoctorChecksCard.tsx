import { Badge, type TBadgeTone } from '@/Core/Ui/Badge';
import { Card } from '@/Core/Ui/Card';
import type { TIconName } from '@/Core/Ui/Icons';
import { Row, Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TDoctorCheck, TDoctorStatus } from '@/Modules/System/Types/SystemTypes';

const TONES: Record<TDoctorStatus, TBadgeTone> = { ok: 'ok', info: 'info', warn: 'warn', fail: 'bad' };
const ICONS: Record<TDoctorStatus, TIconName> = { ok: 'check', info: 'info', warn: 'alert', fail: 'alert' };

export const DoctorBadge = ({ status }: { status: TDoctorStatus }) => {
  const t = useT();

  return (
    <Badge tone={TONES[status]} icon={ICONS[status]}>
      {t(`about.status.${status}`)}
    </Badge>
  );
};

export const DoctorChecksCard = ({ checks }: { checks: TDoctorCheck[] }) => {
  const t = useT();

  return (
    <Card title={t('about.checks.title')} icon="server">
      <Rows label={t('about.checks.title')}>
        {checks.map((check) => (
          <Row
            key={check.Code}
            title={check.Title}
            sub={
              check.Fix ? (
                <>
                  <span className="text-primary">{t('api.error.fix')}</span> {check.Fix}
                </>
              ) : undefined
            }
            end={<DoctorBadge status={check.Status} />}
          />
        ))}
      </Rows>
      <p className="m-0 border-t border-line px-card py-3 text-body-sm text-secondary">{t('about.checks.hostNote')}</p>
    </Card>
  );
};

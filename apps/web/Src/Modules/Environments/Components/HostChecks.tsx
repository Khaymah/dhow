import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Row, Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { THostCheck, TTargetCheck } from '@/Modules/Environments/Api/TargetsApi';
import { checkTone, formatSize } from '@/Modules/Environments/Types/TargetState';

export interface IHostChecksProps {
  check: TTargetCheck;
}

const HostResult = ({ host }: { host: THostCheck }) => {
  const t = useT();
  if (!host.Checked || host.Reason === undefined) return <Badge tone="mute">{t('targets.check.never')}</Badge>;
  return (
    <Badge tone={checkTone(host.Reason)} icon={host.Reason === 'ok' ? 'check' : 'alert'}>
      {t(`targets.check.reason.${host.Reason}`)}
    </Badge>
  );
};

const factsOf = (host: THostCheck, t: ReturnType<typeof useT>, ago: (iso: string) => string): string[] => {
  const facts: string[] = [];
  if (host.CurrentReleaseId) facts.push(t('targets.check.current', { release: host.CurrentReleaseId }));
  if (host.VerifiedReleaseId) facts.push(t('targets.check.verified', { release: host.VerifiedReleaseId }));
  if (host.FreeBytes !== undefined) facts.push(t('targets.check.free', { size: formatSize(host.FreeBytes) }));
  if (host.Locked) facts.push(t('targets.check.locked'));
  if (host.Repaired) facts.push(t('targets.check.repaired'));
  if (host.CheckedAt) facts.push(t('targets.check.at', { time: ago(host.CheckedAt) }));
  return facts;
};

export const HostChecks = ({ check }: IHostChecksProps) => {
  const t = useT();
  const time = useRelativeTime();

  return (
    <Rows label={t('targets.check.title')}>
      {check.Hosts.map((host) => (
        <Row
          key={host.HostId}
          icon="server"
          mono
          title={`${host.Host}:${String(host.Port)}`}
          sub={host.Message ? [host.Message, ...factsOf(host, t, time.ago)].join(' · ') : factsOf(host, t, time.ago).join(' · ')}
          end={<HostResult host={host} />}
        />
      ))}
    </Rows>
  );
};

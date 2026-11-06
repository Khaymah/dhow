import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Button } from '@/Core/Ui/Button';
import { Row, Rows } from '@/Core/Ui/Row';
import { StatusDot } from '@/Core/Ui/StatusDot';
import { useT } from '@/I18n/useT';
import type { TTargetRelease, TTargetReleases } from '@/Modules/Environments/Api/TargetsApi';
import { canRollBackTo, releaseStatusOf } from '@/Modules/Environments/Types/TargetState';

export interface IReleasesListProps {
  releases: TTargetReleases;
  partial: boolean;
  canRollBack: boolean;
  busy: boolean;
  onRollBack: (release: TTargetRelease) => void;
}

const hostsLine = (release: TTargetRelease, t: ReturnType<typeof useT>): string =>
  release.Hosts.map((host) => `${t('targets.release.hostRow', { position: host.Position })}: ${t(`targets.release.host.${host.State}`)}`).join(' · ');

export const ReleasesList = ({ releases, partial, canRollBack, busy, onRollBack }: IReleasesListProps) => {
  const t = useT();
  const time = useRelativeTime();

  return (
    <Rows label={t('targets.releases.title')}>
      {releases.Releases.map((release) => (
        <Row
          key={release.Id}
          icon={release.Kind === 'rollback' ? 'swap' : 'layers'}
          mono
          title={release.ReleaseId}
          sub={
            <>
              {release.Kind === 'rollback' && release.RestoredReleaseId
                ? t('targets.release.kind.rollback', { release: release.RestoredReleaseId })
                : t(`targets.release.kind.${release.Kind}`, { release: '' })}
              {' · '}
              {t('targets.release.started', { time: time.ago(release.StartedAt) })}
              {release.Hosts.length > 0 ? ` · ${hostsLine(release, t)}` : ''}
            </>
          }
          end={
            <>
              <StatusDot status={releaseStatusOf(release.State)}>{t(`targets.release.state.${release.State}`)}</StatusDot>
              {canRollBack && canRollBackTo(release, releases.CurrentReleaseId, partial) ? (
                <Button
                  size="sm"
                  variant="danger"
                  iconStart="swap"
                  disabled={busy}
                  onClick={() => {
                    onRollBack(release);
                  }}
                >
                  {t('targets.rollback.action')}
                </Button>
              ) : null}
            </>
          }
        />
      ))}
    </Rows>
  );
};

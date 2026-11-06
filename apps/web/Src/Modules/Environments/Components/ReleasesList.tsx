import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Row, Rows } from '@/Core/Ui/Row';
import { StatusDot } from '@/Core/Ui/StatusDot';
import { useT } from '@/I18n/useT';
import type { TReleaseHost, TTargetRelease, TTargetReleases } from '@/Modules/Environments/Api/TargetsApi';
import {
  canRollBackTo,
  hostStatusOf,
  isAutomaticRollback,
  isFailedRollback,
  releaseStatusOf,
  rollbackReasonOf,
} from '@/Modules/Environments/Types/TargetState';

export interface IReleasesListProps {
  releases: TTargetReleases;
  partial: boolean;
  canRollBack: boolean;
  busy: boolean;
  onRollBack: (release: TTargetRelease) => void;
  detailed?: boolean;
  hostName?: (hostId: string, position: number) => string;
}

type TTranslate = ReturnType<typeof useT>;

const reasonText = (reason: string | undefined, t: TTranslate): string | null => {
  const known = rollbackReasonOf(reason);
  return known === null ? null : t(`targets.reason.${known}`);
};

const hostsLine = (release: TTargetRelease, t: TTranslate): string =>
  release.Hosts.map((host) => `${t('targets.release.hostRow', { position: host.Position })}: ${t(`targets.release.host.${host.State}`)}`).join(' · ');

const HostDetail = ({ host, name, t }: { host: TReleaseHost; name: string; t: TTranslate }) => {
  const reason = reasonText(host.RollbackReason, t);
  const note = [reason, host.Message].filter((part): part is string => part !== null && part !== undefined && part !== '').join(' · ');
  return (
    <span data-host-state={host.State} className="flex min-w-0 flex-wrap items-center gap-x-3">
      <bdi dir="ltr" className="font-mono">
        {name}
      </bdi>
      <StatusDot status={hostStatusOf(host.State)}>{t(`targets.release.host.${host.State}`)}</StatusDot>
      {note ? <span>{note}</span> : null}
    </span>
  );
};

const Details = ({ release, hostName, t }: { release: TTargetRelease; hostName: IReleasesListProps['hostName']; t: TTranslate }) => (
  <span className="mt-1 grid gap-0.5">
    {release.Hosts.map((host) => (
      <HostDetail
        key={host.HostId}
        host={host}
        name={hostName ? hostName(host.HostId, host.Position) : t('targets.release.hostRow', { position: host.Position })}
        t={t}
      />
    ))}
  </span>
);

const subOf = (release: TTargetRelease, props: IReleasesListProps, t: TTranslate, ago: (iso: string) => string) => {
  const reason = reasonText(release.RollbackReason, t);
  const kind =
    release.Kind === 'rollback' && release.RestoredReleaseId
      ? t('targets.release.kind.rollback', { release: release.RestoredReleaseId })
      : t(`targets.release.kind.${release.Kind}`, { release: '' });
  return (
    <>
      {kind}
      {' · '}
      {t('targets.release.started', { time: ago(release.StartedAt) })}
      {release.FinishedAt ? ` · ${t('targets.release.finished', { time: ago(release.FinishedAt) })}` : ''}
      {reason ? ` · ${t('targets.release.reason', { reason })}` : ''}
      {props.detailed ? null : release.Hosts.length > 0 ? ` · ${hostsLine(release, t)}` : ''}
      {props.detailed && release.Message ? <span className="block">{release.Message}</span> : null}
      {props.detailed && release.Hosts.length > 0 ? <Details release={release} hostName={props.hostName} t={t} /> : null}
    </>
  );
};

const StateBadges = ({ release }: { release: TTargetRelease }) => {
  const t = useT();
  if (isAutomaticRollback(release)) {
    return (
      <Badge tone="bad" icon="alert">
        {t('targets.release.auto')}
      </Badge>
    );
  }
  if (isFailedRollback(release)) {
    return (
      <Badge tone="bad" icon="alert">
        {t('targets.release.rollbackFailed')}
      </Badge>
    );
  }
  return null;
};

export const ReleasesList = (props: IReleasesListProps) => {
  const { releases, partial, canRollBack, busy, onRollBack } = props;
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
          sub={subOf(release, props, t, time.ago)}
          end={
            <>
              <StateBadges release={release} />
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

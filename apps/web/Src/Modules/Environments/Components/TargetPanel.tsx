import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import { useAction } from '@/Core/Hooks/useAction';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { Icon } from '@/Core/Ui/Icon';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import {
  startTargetCheck,
  targetReleasesKey,
  type TTarget,
  type TTargetRelease,
} from '@/Modules/Environments/Api/TargetsApi';
import { CheckHistoryRow } from '@/Modules/Environments/Components/CheckHistoryRow';
import { HostChecks } from '@/Modules/Environments/Components/HostChecks';
import { ReleasesList } from '@/Modules/Environments/Components/ReleasesList';
import { RollbackPanel } from '@/Modules/Environments/Components/RollbackPanel';
import { useTargetCheck, useTargetReleases } from '@/Modules/Environments/Hooks/useTargets';
import { CHECK_WATCH_MS, checkedSince, differentReleases } from '@/Modules/Environments/Types/TargetState';

export interface ITargetPanelProps {
  organizationId: string;
  projectId: string;
  environmentName: string;
  target: TTarget;
  canAct: boolean;
  paused: boolean;
  detailed?: boolean;
  manageHref?: string;
  onManage?: () => void;
}

export const CheckSection = ({ organizationId, projectId, environmentName, target, canAct }: ITargetPanelProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const client = useQueryClient();
  const Push = useToastStore((state) => state.Push);
  const [Since, setSince] = useState<number | null>(null);
  const [Expired, setExpired] = useState(false);
  const check = useTargetCheck(organizationId, projectId, environmentName, target.Name, Since);
  const start = useAction(() => startTargetCheck(organizationId, projectId, environmentName, target.Name));
  const Done = Since !== null && checkedSince(check.data, Since);
  const Waiting = Since !== null && !Expired && !Done;

  useEffect(() => {
    if (Since === null) return undefined;
    const timer = window.setTimeout(() => {
      setExpired(true);
    }, CHECK_WATCH_MS);
    return () => {
      window.clearTimeout(timer);
    };
  }, [Since]);

  useEffect(() => {
    if (Done) void client.invalidateQueries({ queryKey: targetReleasesKey(organizationId, projectId, environmentName, target.Name) });
  }, [Done, client, organizationId, projectId, environmentName, target.Name]);

  const HandleCheck = () => {
    void start.Run().then((result) => {
      if (isApiError(result)) return;
      setExpired(false);
      setSince(Date.now());
      void check.refetch();
      Push({ Id: `target-check-${result.RunId}`, Accent: 'building', Title: t('targets.check.started', { name: target.Name }) });
    });
  };

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-3 px-card">
        <h4 className="m-0 text-label">{t('targets.check.title')}</h4>
        {canAct ? (
          <Button size="sm" iconStart="check" disabled={start.Pending || Waiting} onClick={HandleCheck}>
            {start.Pending ? t('targets.check.busy') : t('targets.check.button')}
          </Button>
        ) : null}
      </div>
      {start.Failure ? (
        <div className="px-card">
          <InlineAlert accent="failed" title={errorText(start.Failure)} />
        </div>
      ) : null}
      {Waiting ? (
        <div className="px-card">
          <InlineAlert accent="building" title={t('targets.check.waiting')} />
        </div>
      ) : null}
      <CardStates
        state={cardStateOf(check, false)}
        loadingLabel={t('targets.check.loading')}
        errorTitle={t('targets.check.error')}
        errorText={check.error ? errorText(check.error) : undefined}
        retryLabel={t('status.retry')}
        onRetry={() => {
          void check.refetch();
        }}
        emptyIcon="server"
        emptyText={t('targets.check.never')}
        lines={3}
      >
        {check.data ? <HostChecks check={check.data} /> : null}
      </CardStates>
    </div>
  );
};

export const ReleasesSection = ({ organizationId, projectId, environmentName, target, canAct, paused, detailed = false }: ITargetPanelProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const releases = useTargetReleases(organizationId, projectId, environmentName, target.Name);
  const check = useTargetCheck(organizationId, projectId, environmentName, target.Name, null);
  const [Asking, setAsking] = useState<TTargetRelease | null>(null);
  const data = releases.data;

  return (
    <div className="grid gap-3">
      <h4 className="m-0 px-card text-label">{t('targets.releases.title')}</h4>
      {paused && canAct ? (
        <div className="px-card">
          <InlineAlert accent="awaiting" title={t('targets.paused')} />
        </div>
      ) : null}
      {detailed ? <CheckHistoryRow check={check.data} /> : null}
      <CardStates
        state={cardStateOf(releases, data?.Releases.length === 0)}
        loadingLabel={t('targets.releases.loading')}
        errorTitle={t('targets.releases.error')}
        errorText={releases.error ? errorText(releases.error) : undefined}
        retryLabel={t('status.retry')}
        onRetry={() => {
          void releases.refetch();
        }}
        emptyIcon="layers"
        emptyText={t('targets.releases.empty')}
        lines={3}
      >
        {data ? (
          <ReleasesList
            releases={data}
            partial={data.Partial}
            canRollBack={canAct && !paused}
            busy={Asking !== null}
            onRollBack={setAsking}
            detailed={detailed}
            hostName={(hostId, position) => {
              const host = target.Hosts.find((item) => item.Id === hostId);
              return host ? `${host.Host}:${String(host.Port)}` : t('targets.release.hostRow', { position });
            }}
          />
        ) : null}
      </CardStates>
      {Asking ? (
        <FactorGate>
          {(methods) => (
            <RollbackPanel
              organizationId={organizationId}
              projectId={projectId}
              environmentName={environmentName}
              target={target}
              release={Asking}
              methods={methods}
              onClose={() => {
                setAsking(null);
              }}
            />
          )}
        </FactorGate>
      ) : null}
    </div>
  );
};

const hooksHidden = (target: TTarget): boolean => {
  const ssh = target.Settings.Ssh;
  if (ssh === undefined) return false;
  return (
    (ssh.ReloadCommandSet === true && ssh.ReloadCommand === undefined) ||
    (ssh.PreSwitchCommandSet === true && ssh.PreSwitchCommand === undefined) ||
    (ssh.PostSwitchCommandSet === true && ssh.PostSwitchCommand === undefined)
  );
};

export const Header = ({ target, current, different }: { target: TTarget; current: string | undefined; different: boolean }) => {
  const t = useT();
  return (
    <div className="grid gap-2 px-card">
      <p className="m-0 flex flex-wrap items-center gap-2 text-body-sm text-secondary">
        <span>{t(`targets.kind.${target.Kind}`)}</span>
        <span>{t('targets.hosts', { count: target.Hosts.length })}</span>
        <span>{current ? t('targets.current', { release: current }) : t('targets.current.none')}</span>
      </p>
      {hooksHidden(target) ? <p className="m-0 text-body-sm text-tertiary">{t('targets.hooks.hidden')}</p> : null}
      {target.Ready ? null : <InlineAlert accent="awaiting" title={t('targets.notReady')} />}
      {different ? <InlineAlert accent="failed" title={t('targets.partial')}>{t('targets.partial.hint')}</InlineAlert> : null}
    </div>
  );
};

export const TargetPanel = (props: ITargetPanelProps) => {
  const t = useT();
  const { organizationId, projectId, environmentName, target } = props;
  const releases = useTargetReleases(organizationId, projectId, environmentName, target.Name);
  const check = useTargetCheck(organizationId, projectId, environmentName, target.Name, null);
  const different = differentReleases(releases.data, check.data);

  return (
    <Card
      title={target.Name}
      icon="server"
      end={
        <span className="flex flex-wrap items-center gap-3">
          {different ? (
            <Badge tone="bad" icon="alert">
              {t('targets.partial')}
            </Badge>
          ) : null}
          {props.manageHref ? (
            <a
              href={props.manageHref}
              onClick={followInApp(() => {
                props.onManage?.();
              })}
              className="inline-flex min-h-11 items-center gap-1.5 rounded-xs text-label-sm text-secondary no-underline hover:text-primary"
            >
              {t('targets.manage')}
              <Icon name="next" size={14} />
            </a>
          ) : null}
        </span>
      }
    >
      <div className="grid gap-4 pb-card">
        <Header target={target} current={releases.data?.CurrentReleaseId} different={different} />
        <CheckSection {...props} />
        <ReleasesSection {...props} />
      </div>
    </Card>
  );
};

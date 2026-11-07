import { cx } from '@/Core/Ui/ClassNames';
import { StatTile, type IStatTileProps } from '@/Core/Ui/StatTile';
import { useT } from '@/I18n/useT';
import type { TOverview } from '@/Modules/Overview/Api/OverviewApi';
import type { IOverviewLinks, TOverviewTarget } from '@/Modules/Overview/Types/OverviewTargets';

export interface IOverviewStatsProps {
  overview: TOverview;
  links: IOverviewLinks;
}

type TStat = Omit<IStatTileProps, 'href' | 'onSelect'> & { target: TOverviewTarget };

const COLUMNS: Record<number, string> = {
  1: 'lg:grid-cols-1',
  2: 'lg:grid-cols-2',
  3: 'lg:grid-cols-3',
  4: 'lg:grid-cols-4',
  5: 'lg:grid-cols-5',
};

export const OverviewStats = ({ overview, links }: IOverviewStatsProps) => {
  const t = useT();
  const { Jobs: jobs, Runners: runners, Deployments: deployments } = overview;
  const stats: TStat[] = [];
  if (jobs) {
    stats.push(
      { target: 'running', value: jobs.Running, label: t('home.stat.running'), icon: 'play', tone: 'info' },
      {
        target: 'queued',
        value: jobs.Queued,
        label: t('home.stat.queued'),
        icon: 'queue',
        tone: 'queue',
        sub: jobs.QueuedNoRunner > 0 ? t('home.stat.queued.stuck', { count: jobs.QueuedNoRunner }) : undefined,
      },
      { target: 'failed', value: jobs.FailedLast24h, label: t('home.stat.failed'), icon: 'alert', tone: 'bad' },
    );
  }
  if (runners) {
    stats.push({
      target: 'runners',
      value: runners.Online,
      label: t('home.stat.online'),
      icon: 'server',
      tone: 'ok',
      sub: runners.Offline > 0 ? t('home.stat.online.offline', { count: runners.Offline }) : undefined,
    });
  }
  if (deployments) {
    stats.push({
      target: 'deploys',
      value: deployments.Running,
      label: t('home.stat.deploys'),
      icon: 'layers',
      tone: 'info',
      sub: deployments.FailedLast24h > 0 ? t('home.stat.deploys.failed', { count: deployments.FailedLast24h }) : undefined,
    });
  }
  if (stats.length === 0) return null;

  return (
    <div className={cx('grid grid-cols-2 gap-3', COLUMNS[stats.length])}>
      {stats.map(({ target, ...stat }) => (
        <StatTile
          key={target}
          {...stat}
          href={links.href(target)}
          onSelect={() => {
            links.open(target);
          }}
        />
      ))}
    </div>
  );
};

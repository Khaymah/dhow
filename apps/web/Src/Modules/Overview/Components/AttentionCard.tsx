import type { UseQueryResult } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Badge, type TBadgeTone } from '@/Core/Ui/Badge';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import { Icon } from '@/Core/Ui/Icon';
import type { TIconName } from '@/Core/Ui/Icons';
import { Row, Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TMessageKey } from '@/I18n/Messages';
import type { TOverview } from '@/Modules/Overview/Api/OverviewApi';
import type { IOverviewLinks, TOverviewTarget } from '@/Modules/Overview/Types/OverviewTargets';

export interface IAttentionCardProps {
  overview: UseQueryResult<TOverview, ApiError>;
  failingChecks: number;
  links: IOverviewLinks;
  className?: string;
}

interface IAttention {
  target: TOverviewTarget;
  icon: TIconName;
  tone: TBadgeTone;
  count: number;
  title: TMessageKey;
  sub: TMessageKey;
}

const attentionOf = (overview: TOverview | undefined, failingChecks: number): IAttention[] => {
  const all: IAttention[] = [
    {
      target: 'approvals', icon: 'shield', tone: 'warn', count: overview?.Approvals?.Pending ?? 0,
      title: 'home.attention.approvals.title', sub: 'home.attention.approvals.sub',
    },
    {
      target: 'stuck', icon: 'queue', tone: 'warn', count: overview?.Jobs?.QueuedNoRunner ?? 0,
      title: 'home.attention.stuck.title', sub: 'home.attention.stuck.sub',
    },
    {
      target: 'checks', icon: 'shield', tone: 'bad', count: failingChecks,
      title: 'home.attention.checks.title', sub: 'home.attention.checks.sub',
    },
    {
      target: 'offline', icon: 'server', tone: 'warn', count: overview?.Runners?.Offline ?? 0,
      title: 'home.attention.offline.title', sub: 'home.attention.offline.sub',
    },
    {
      target: 'invitations', icon: 'mail', tone: 'mute', count: overview?.Invitations?.Pending ?? 0,
      title: 'home.attention.invitations.title', sub: 'home.attention.invitations.sub',
    },
  ];
  return all.filter((item) => item.count > 0);
};

export const AttentionCard = ({ overview, failingChecks, links, className }: IAttentionCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const items = attentionOf(overview.data, failingChecks);
  const state = cardStateOf(overview, items.length === 0);

  return (
    <Card
      title={t('home.attention.title')}
      icon="bell"
      className={className}
      end={items.length > 0 ? <Badge tone="mute">{items.length}</Badge> : null}
    >
      {state === 'empty' ? (
        <p className="m-0 flex items-center gap-2.5 px-card pt-1 pb-card text-body-sm text-secondary">
          <Icon name="check" size={18} className="flex-none text-ok-fg" />
          {t('home.attention.calm')}
        </p>
      ) : (
        <CardStates
          state={state}
          loadingLabel={t('home.attention.loading')}
          errorTitle={t('home.overview.error')}
          errorText={overview.error ? errorText(overview.error) : undefined}
          retryLabel={t('jobs.retry')}
          onRetry={() => {
            void overview.refetch();
          }}
          emptyIcon="check"
          emptyText={t('home.attention.calm')}
        >
          <Rows label={t('home.attention.title')}>
            {items.map((item) => (
              <Row
                key={item.target}
                icon={item.icon}
                title={t(item.title)}
                sub={t(item.sub)}
                end={<Badge tone={item.tone}>{item.count}</Badge>}
                onSelect={() => {
                  links.open(item.target);
                }}
              />
            ))}
          </Rows>
        </CardStates>
      )}
    </Card>
  );
};

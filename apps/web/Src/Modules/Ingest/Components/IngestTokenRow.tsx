import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge, type TBadgeTone } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Icon } from '@/Core/Ui/Icon';
import { useT } from '@/I18n/useT';
import type { TIngestToken } from '@/Modules/Ingest/Api/IngestApi';
import { ReachList } from '@/Modules/Ingest/Components/ReachList';

const TONES: Record<TIngestToken['Status'], TBadgeTone> = { active: 'ok', expired: 'mute', revoked: 'mute' };

export interface IIngestTokenRowProps {
  token: TIngestToken;
  creator: string;
  onAskRevoke?: (() => void) | undefined;
}

export const IngestTokenRow = ({ token, creator, onAskRevoke }: IIngestTokenRowProps) => {
  const t = useT();
  const time = useRelativeTime();
  const live = token.Status === 'active';
  const facts = [
    t('ingest.token.createdBy', { name: creator }),
    live ? t('ingest.token.expires', { when: time.full(token.ExpiresAt) }) : t('ingest.token.expired', { when: time.full(token.ExpiresAt) }),
    token.LastUsedAt ? t('ingest.token.lastUsed', { when: time.ago(token.LastUsedAt) }) : t('ingest.token.neverUsed'),
  ];

  return (
    <li className="grid min-w-0 gap-2 border-t border-line px-card py-3 first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Icon name="key" size={18} className="flex-none text-secondary" />
        <span className="grid min-w-0 flex-1 gap-0.5">
          <span className="min-w-0 text-label">{token.Name}</span>
          <small className="min-w-0 text-body-sm text-tertiary">
            <bdi dir="ltr" className="font-mono">
              {token.Prefix}
            </bdi>
          </small>
        </span>
        <Badge tone={TONES[token.Status]}>{t(`ingest.token.status.${token.Status}`)}</Badge>
        {live && onAskRevoke ? (
          <Button size="sm" variant="ghost" iconStart="trash" aria-label={t('ingest.revoke.aria', { name: token.Name })} onClick={onAskRevoke}>
            {t('ingest.revoke')}
          </Button>
        ) : null}
      </div>
      <ul aria-label={t('ingest.patterns.title')} className="m-0 flex list-none flex-wrap gap-1.5 p-0">
        {token.RefPatterns.map((item) => (
          <li key={`${item.Kind}:${item.Pattern}`}>
            <Badge tone="mute">
              {t(`ingest.kind.${item.Kind}`)}
              <bdi dir="ltr" className="font-mono">
                {item.Pattern}
              </bdi>
            </Badge>
          </li>
        ))}
      </ul>
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <span className="text-body-sm text-secondary">{t('ingest.reach.label')}</span>
        <ReachList names={token.Reaches ?? []} />
      </div>
      <small className="text-body-sm text-tertiary">{facts.join(' · ')}</small>
    </li>
  );
};

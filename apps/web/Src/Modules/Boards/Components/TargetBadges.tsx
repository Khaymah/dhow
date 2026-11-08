import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { useT } from '@/I18n/useT';
import type { TBoardTarget } from '@/Modules/Boards/Api/BoardsApi';
import { lookOf } from '@/Modules/Boards/Types/BoardStatus';

export const LastStatus = ({ target }: { target: TBoardTarget }) => {
  const t = useT();
  const time = useRelativeTime();
  if (target.Running !== undefined) {
    const look = lookOf('running');
    return (
      <Badge tone={look.Tone} icon={look.Icon}>
        {t(look.Label)}
      </Badge>
    );
  }
  if (target.Last === undefined) return <small className="text-body-sm text-tertiary">{t('boards.last.none')}</small>;
  const look = lookOf(target.Last.State);
  return (
    <span className="flex flex-wrap items-center gap-2">
      {target.RollbackFailed ? null : (
        <Badge tone={look.Tone} icon={look.Icon}>
          {t(look.Label)}
        </Badge>
      )}
      <small className="text-body-sm text-tertiary">{t('boards.last.at', { time: time.ago(target.Last.StartedAt) })}</small>
    </span>
  );
};

export const AttentionBadges = ({ target }: { target: TBoardTarget }) => {
  const t = useT();
  if (!target.RollbackFailed && !target.Partial) return null;
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {target.RollbackFailed ? (
        <Badge tone="bad" icon="alert">
          {t('boards.badge.rollbackFailed')}
        </Badge>
      ) : null}
      {target.Partial ? (
        <Badge tone="warn" icon="layers">
          {t('boards.badge.partial')}
        </Badge>
      ) : null}
    </span>
  );
};

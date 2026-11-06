import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { Badge } from '@/Core/Ui/Badge';
import { Row, Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TTargetCheck } from '@/Modules/Environments/Api/TargetsApi';
import { lastCheckOf } from '@/Modules/Environments/Types/TargetState';

export interface ICheckHistoryRowProps {
  check: TTargetCheck | undefined;
}

export const CheckHistoryRow = ({ check }: ICheckHistoryRowProps) => {
  const t = useT();
  const time = useRelativeTime();
  const last = lastCheckOf(check);
  if (last === null) return null;

  return (
    <Rows label={t('targets.history.kind.check')}>
      <Row
        icon="check"
        title={t('targets.history.kind.check')}
        sub={t('targets.check.at', { time: time.ago(last.At) })}
        end={
          last.Problems === 0 ? (
            <Badge tone="ok" icon="check">
              {t('targets.history.check.ok')}
            </Badge>
          ) : (
            <Badge tone="bad" icon="alert">
              {t('targets.history.check.problems', { count: last.Problems })}
            </Badge>
          )
        }
      />
    </Rows>
  );
};

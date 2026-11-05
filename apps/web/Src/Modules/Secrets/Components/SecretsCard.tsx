import type { UseQueryResult } from '@tanstack/react-query';
import type { ReactNode } from 'react';

import type { ApiError } from '@/Core/Api/ApiError';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { CardStates, cardStateOf } from '@/Core/Ui/CardStates';
import type { TIconName } from '@/Core/Ui/Icons';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Rows } from '@/Core/Ui/Row';
import { useT } from '@/I18n/useT';
import type { TMessageKey } from '@/I18n/Messages';

export interface ISecretsCardLabels {
  list: TMessageKey;
  loading: TMessageKey;
  error: TMessageKey;
  empty: TMessageKey;
  add: TMessageKey;
}

export interface ISecretsCardProps<TItem> {
  icon: TIconName;
  labels: ISecretsCardLabels;
  query: UseQueryResult<TItem[], ApiError>;
  canAdd: boolean;
  primary?: boolean;
  notice?: string | null;
  onAdd: () => void;
  children: (items: TItem[]) => ReactNode;
}

export const SecretsCard = <TItem,>(props: ISecretsCardProps<TItem>) => {
  const { icon, labels, query, canAdd, primary = false, notice = null, onAdd, children } = props;
  const t = useT();
  const errorText = useApiErrorText();
  const items = query.data;
  const variant = primary ? 'primary' : 'secondary';

  return (
    <Card title={t(labels.list)} icon={icon} end={items === undefined ? undefined : t('secrets.count', { count: items.length })}>
      {notice ? (
        <div className="px-card pb-3">
          <InlineAlert accent="failed" title={notice} />
        </div>
      ) : null}
      <CardStates
        state={cardStateOf(query, items?.length === 0)}
        loadingLabel={t(labels.loading)}
        errorTitle={t(labels.error)}
        errorText={query.error ? errorText(query.error) : undefined}
        retryLabel={t('status.retry')}
        onRetry={() => {
          void query.refetch();
        }}
        emptyIcon={icon}
        emptyText={t(labels.empty)}
        emptyAction={
          canAdd ? (
            <Button size="sm" variant={variant} iconStart="plus" onClick={onAdd}>
              {t(labels.add)}
            </Button>
          ) : undefined
        }
        lines={4}
      >
        <Rows label={t(labels.list)}>{items ? children(items) : null}</Rows>
        {canAdd ? (
          <div className="flex border-t border-line px-card py-3">
            <Button size="sm" variant={variant} iconStart="plus" onClick={onAdd}>
              {t(labels.add)}
            </Button>
          </div>
        ) : null}
      </CardStates>
    </Card>
  );
};

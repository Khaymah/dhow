import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { Field } from '@/Core/Ui/Field';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Pager } from '@/Core/Ui/Pager';
import { Select } from '@/Core/Ui/Select';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { DELIVERIES_PAGE_SIZE, type TDeliveryState } from '@/Modules/Notifications/Api/NotificationsApi';
import { DeliveryRow } from '@/Modules/Notifications/Components/DeliveryRow';
import { useDeliveries } from '@/Modules/Notifications/Hooks/useNotifications';
import { DELIVERY_STATES, stateKey } from '@/Modules/Notifications/Types/DeliveryCodes';

export interface IDeliveriesCardProps {
  organizationId: string;
  projectId: string;
}

const ALL = 'all';

export const DeliveriesCard = ({ organizationId, projectId }: IDeliveriesCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Page, setPage] = useState(1);
  const [State, setState] = useState<TDeliveryState | undefined>(undefined);
  const deliveries = useDeliveries(organizationId, projectId, Page, State);

  const filter = (
    <div className="px-card pb-2">
      <Field label={t('notifications.deliveries.filter')}>
        <Select
          compact
          value={State ?? ALL}
          options={[
            { value: ALL, label: t('notifications.deliveries.state.all') },
            ...DELIVERY_STATES.map((state) => ({ value: state, label: t(stateKey(state)) })),
          ]}
          onChange={(event) => {
            setState(DELIVERY_STATES.find((state) => state === event.target.value));
            setPage(1);
          }}
        />
      </Field>
    </div>
  );

  if (deliveries.isPending) return <Skeleton label={t('notifications.deliveries.loading')} lines={3} />;
  if (deliveries.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('notifications.deliveries.error')}>
          {errorText(deliveries.error)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void deliveries.refetch();
          }}
        >
          {t('projects.retry')}
        </Button>
      </div>
    );
  }
  const { Items, Meta } = deliveries.data;

  return (
    <Card title={t('notifications.deliveries.title')} icon="file" end={Meta.Total > 0 ? String(Meta.Total) : undefined}>
      <p className="m-0 px-card pb-2 text-body-sm text-secondary">{t('notifications.deliveries.lead')}</p>
      {filter}
      {Items.length === 0 ? <p className="m-0 px-card pb-3 text-body-sm text-secondary">{t('notifications.deliveries.empty')}</p> : null}
      <ul aria-label={t('notifications.deliveries.title')} className="m-0 grid list-none p-0">
        {Items.map((delivery) => (
          <DeliveryRow key={delivery.Id} organizationId={organizationId} projectId={projectId} delivery={delivery} />
        ))}
      </ul>
      <div className="grid gap-3 p-card pt-2">
        <Pager page={Page} limit={DELIVERIES_PAGE_SIZE} total={Meta.Total} busy={deliveries.isFetching} onPage={setPage} />
      </div>
    </Card>
  );
};

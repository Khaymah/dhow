import { CheckList } from '@/Core/Ui/CheckList';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { ReachList } from '@/Modules/Ingest/Components/ReachList';

export interface IIngestReachConfirmProps {
  names: readonly string[];
  acknowledged: boolean;
  disabled: boolean;
  onChange: (acknowledged: boolean) => void;
}

const ACKNOWLEDGE = 'acknowledge';

export const IngestReachConfirm = ({ names, acknowledged, disabled, onChange }: IIngestReachConfirmProps) => {
  const t = useT();

  return (
    <section className="grid gap-3 rounded-md border border-warn-line bg-warn-wash p-card">
      <InlineAlert accent="awaiting" title={t('ingest.reach.title')}>
        {t('ingest.reach.lead')}
      </InlineAlert>
      <ReachList names={names} tone="warn" />
      <CheckList
        label={t('ingest.reach.group')}
        items={[{ value: ACKNOWLEDGE, title: t('ingest.reach.acknowledge'), about: t('ingest.reach.acknowledge.about') }]}
        values={acknowledged ? [ACKNOWLEDGE] : []}
        disabled={disabled}
        onChange={(values) => {
          onChange(values.includes(ACKNOWLEDGE));
        }}
      />
      <small className="text-body-sm text-secondary">{t('ingest.reach.again')}</small>
    </section>
  );
};

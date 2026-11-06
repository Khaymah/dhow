import { useT } from '@/I18n/useT';
import { Chip } from '@/Modules/Ingest/Components/Chip';

export interface IReachListProps {
  names: readonly string[];
  tone?: 'mute' | 'warn';
}

export const ReachList = ({ names, tone = 'mute' }: IReachListProps) => {
  const t = useT();
  if (names.length === 0) return <span className="text-body-sm text-tertiary">{t('ingest.reach.none')}</span>;
  return (
    <ul aria-label={t('ingest.reach.label')} className="m-0 flex min-w-0 list-none flex-wrap gap-1.5 p-0">
      {names.map((name) => (
        <li key={name} className="min-w-0 max-w-full">
          <Chip tone={tone} protectedMark={tone === 'warn'}>
            <bdi dir="ltr" className="font-mono">
              {name}
            </bdi>
          </Chip>
        </li>
      ))}
    </ul>
  );
};

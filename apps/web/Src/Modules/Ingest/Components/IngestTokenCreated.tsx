import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { copyText } from '@/Core/Ui/Clipboard';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TCreatedIngestToken } from '@/Modules/Ingest/Api/IngestApi';
import { ReachList } from '@/Modules/Ingest/Components/ReachList';

export interface IIngestTokenCreatedProps {
  created: TCreatedIngestToken;
  onDone: () => void;
}

export const IngestTokenCreated = ({ created, onDone }: IIngestTokenCreatedProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const reaches = created.Token.Reaches ?? [];

  return (
    <div className="grid gap-4">
      <InlineAlert accent="awaiting" title={t('ingest.created.title')}>
        {t('ingest.created.lead')}
      </InlineAlert>
      <code
        dir="ltr"
        aria-label={t('ingest.created.value')}
        className="block rounded-md border border-line bg-surface-inset px-3 py-2.5 text-code break-all select-all"
      >
        {created.Value}
      </code>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="primary"
          iconStart="copy"
          onClick={() => {
            void copyText(created.Value).then((done) => {
              Push(
                done
                  ? { Id: 'ingest-token-copied', Accent: 'live', Title: t('ingest.created.copied') }
                  : { Id: 'ingest-token-copy-failed', Accent: 'awaiting', Title: t('ingest.created.copyFailed') },
              );
            });
          }}
        >
          {t('ingest.created.copy')}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          {t('ingest.created.done')}
        </Button>
      </div>
      <p className="m-0 text-body-sm text-secondary">{t('ingest.created.usage')}</p>
      {reaches.length > 0 ? (
        <div className="grid gap-1.5">
          <span className="text-body-sm text-secondary">{t('ingest.created.reaches')}</span>
          <ReachList names={reaches} />
        </div>
      ) : null}
    </div>
  );
};

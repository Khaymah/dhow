import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { copyText } from '@/Core/Ui/Clipboard';
import { useI18n } from '@/I18n/useI18n';
import { ACTION_SNIPPET, CLI_SNIPPET, INGEST_ACTION_URL, INGEST_GUIDE_URL } from '@/Modules/Ingest/Types/IngestForm';

interface ISnippetProps {
  id: string;
  title: string;
  value: string;
}

const Snippet = ({ id, title, value }: ISnippetProps) => {
  const { t } = useI18n();
  const Push = useToastStore((state) => state.Push);

  return (
    <div className="grid min-w-0 gap-2">
      <span className="text-label">{title}</span>
      <pre dir="ltr" className="m-0 overflow-x-auto rounded-md border border-line bg-surface-inset px-3 py-2.5 text-code">
        <code>{value}</code>
      </pre>
      <Button
        size="sm"
        iconStart="copy"
        className="justify-self-start"
        onClick={() => {
          void copyText(value).then((done) => {
            Push(
              done
                ? { Id: `ingest-snippet-${id}`, Accent: 'live', Title: t('ingest.send.copied') }
                : { Id: `ingest-snippet-failed-${id}`, Accent: 'awaiting', Title: t('projects.copy.failed') },
            );
          });
        }}
      >
        {t('ingest.send.copy', { title })}
      </Button>
    </div>
  );
};

export const SendBuildPanel = () => {
  const { t, Locale } = useI18n();

  return (
    <Card title={t('ingest.send.title')} icon="terminal" padded>
      <div className="grid gap-4">
        <p className="m-0 text-body-sm text-secondary">{t('ingest.send.lead')}</p>
        <Snippet id="action" title={t('ingest.send.action')} value={ACTION_SNIPPET} />
        <p className="m-0 text-body-sm text-secondary">{t('ingest.send.env')}</p>
        <Snippet id="cli" title={t('ingest.send.cli')} value={CLI_SNIPPET} />
        <p className="m-0 text-body-sm text-secondary">{t('ingest.send.cliToken')}</p>
        <p className="m-0 text-body-sm text-secondary">{t('ingest.send.secret')}</p>
        <div className="flex flex-wrap gap-x-4 gap-y-1">
          <a href={INGEST_GUIDE_URL[Locale]} target="_blank" rel="noreferrer noopener" className="text-body-sm text-primary underline">
            {t('ingest.send.guide')}
          </a>
          <a href={INGEST_ACTION_URL[Locale]} target="_blank" rel="noreferrer noopener" className="text-body-sm text-primary underline">
            {t('ingest.send.actionGuide')}
          </a>
        </div>
      </div>
    </Card>
  );
};

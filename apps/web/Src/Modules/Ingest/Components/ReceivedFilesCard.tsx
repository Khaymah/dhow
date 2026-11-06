import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Pager } from '@/Core/Ui/Pager';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { INGEST_PAGE_SIZE } from '@/Modules/Ingest/Api/IngestApi';
import { ReceivedFileRow } from '@/Modules/Ingest/Components/ReceivedFileRow';
import { useIngestedArtifacts } from '@/Modules/Ingest/Hooks/useIngest';

export interface IReceivedFilesCardProps {
  organizationId: string;
  projectId: string;
  runHref: (runId: string) => string;
  onOpenRun: (runId: string) => void;
}

export const ReceivedFilesCard = ({ organizationId, projectId, runHref, onOpenRun }: IReceivedFilesCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Page, setPage] = useState(1);
  const files = useIngestedArtifacts(organizationId, projectId, Page);

  if (files.isPending) return <Skeleton label={t('ingest.files.loading')} lines={3} />;
  if (files.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('ingest.files.error')}>
          {errorText(files.error)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void files.refetch();
          }}
        >
          {t('projects.retry')}
        </Button>
      </div>
    );
  }
  const { Items, Meta } = files.data;

  return (
    <Card title={t('ingest.files.title')} icon="file" end={Meta.Total > 0 ? String(Meta.Total) : undefined}>
      <p className="m-0 px-card pb-2 text-body-sm text-secondary">{t('ingest.files.lead')}</p>
      {Items.length === 0 ? <p className="m-0 px-card pb-3 text-body-sm text-secondary">{t('ingest.files.empty')}</p> : null}
      <ul aria-label={t('ingest.files.title')} className="m-0 grid list-none p-0">
        {Items.map((file) => (
          <ReceivedFileRow key={file.Id} file={file} runHref={runHref} onOpenRun={onOpenRun} />
        ))}
      </ul>
      <div className="p-card pt-2">
        <Pager page={Page} limit={INGEST_PAGE_SIZE} total={Meta.Total} busy={files.isFetching} onPage={setPage} />
      </div>
    </Card>
  );
};

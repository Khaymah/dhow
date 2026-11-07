import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Pager } from '@/Core/Ui/Pager';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { INGEST_PAGE_SIZE } from '@/Modules/Ingest/Api/IngestApi';
import { IngestTokenAcknowledge } from '@/Modules/Ingest/Components/IngestTokenAcknowledge';
import { IngestTokenRevoke } from '@/Modules/Ingest/Components/IngestTokenRevoke';
import { IngestTokenRow } from '@/Modules/Ingest/Components/IngestTokenRow';
import { useCreatorName } from '@/Modules/Ingest/Hooks/useCreatorName';
import { useIngestTokens } from '@/Modules/Ingest/Hooks/useIngest';

export interface IIngestTokensCardProps {
  organizationId: string;
  projectId: string;
  writable: boolean;
  canConfirm: boolean;
  onNew: () => void;
}

export const IngestTokensCard = ({ organizationId, projectId, writable, canConfirm, onNew }: IIngestTokensCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Page, setPage] = useState(1);
  const [Revoking, setRevoking] = useState<string | null>(null);
  const [Acknowledging, setAcknowledging] = useState<string | null>(null);
  const tokens = useIngestTokens(organizationId, projectId, Page);
  const creator = useCreatorName(organizationId, true);

  if (tokens.isPending) return <Skeleton label={t('ingest.tokens.loading')} lines={3} />;
  if (tokens.isError) {
    return (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('ingest.tokens.error')}>
          {errorText(tokens.error)}
        </InlineAlert>
        <Button
          size="sm"
          onClick={() => {
            void tokens.refetch();
          }}
        >
          {t('projects.retry')}
        </Button>
      </div>
    );
  }
  const { Items, Meta } = tokens.data;

  return (
    <Card title={t('ingest.tokens.title')} icon="key" end={Meta.Total > 0 ? String(Meta.Total) : undefined}>
      <p className="m-0 px-card pb-2 text-body-sm text-secondary">{t('ingest.tokens.lead')}</p>
      {Items.length === 0 ? <p className="m-0 px-card pb-3 text-body-sm text-secondary">{t('ingest.tokens.empty')}</p> : null}
      <ul aria-label={t('ingest.tokens.title')} className="m-0 grid list-none p-0">
        {Items.map((token) => [
          <IngestTokenRow
            key={token.Id}
            token={token}
            creator={creator(token.CreatedByUserId)}
            onAskRevoke={
              writable
                ? () => {
                    setRevoking(token.Id);
                  }
                : undefined
            }
            confirmNeedsAdmin={!canConfirm}
            onAskAcknowledge={
              writable && canConfirm
                ? () => {
                    setAcknowledging(token.Id);
                  }
                : undefined
            }
          />,
          Acknowledging === token.Id ? (
            <IngestTokenAcknowledge
              key={`${token.Id}-ack`}
              organizationId={organizationId}
              projectId={projectId}
              token={token}
              onClose={() => {
                setAcknowledging(null);
              }}
            />
          ) : null,
          Revoking === token.Id ? (
            <IngestTokenRevoke
              key={`${token.Id}-revoke`}
              organizationId={organizationId}
              projectId={projectId}
              token={token}
              onClose={() => {
                setRevoking(null);
              }}
            />
          ) : null,
        ])}
      </ul>
      <div className="grid gap-3 p-card pt-2">
        <Pager page={Page} limit={INGEST_PAGE_SIZE} total={Meta.Total} busy={tokens.isFetching} onPage={setPage} />
        {writable ? (
          <Button className="w-full" iconStart="plus" onClick={onNew}>
            {t('ingest.tokens.new')}
          </Button>
        ) : null}
      </div>
    </Card>
  );
};

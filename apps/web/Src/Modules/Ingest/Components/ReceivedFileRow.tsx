import type { ReactNode } from 'react';

import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Badge } from '@/Core/Ui/Badge';
import { copyText } from '@/Core/Ui/Clipboard';
import { Icon } from '@/Core/Ui/Icon';
import { IconButton } from '@/Core/Ui/IconButton';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';
import type { TIngestedArtifact } from '@/Modules/Ingest/Api/IngestApi';
import { useSizeText } from '@/Modules/Ingest/Hooks/useSizeText';
import { provenanceEntries, shortCommit, shortDigest, shortRefName } from '@/Modules/Ingest/Types/IngestDisplay';

export interface IReceivedFileRowProps {
  file: TIngestedArtifact;
  runHref: (runId: string) => string;
  onOpenRun: (runId: string) => void;
}

const Fact = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex min-w-0 flex-wrap items-center gap-x-2">
    <dt className="text-body-sm text-tertiary">{label}</dt>
    <dd className="m-0 min-w-0 text-body-sm text-secondary">{children}</dd>
  </div>
);

export const ReceivedFileRow = ({ file, runHref, onOpenRun }: IReceivedFileRowProps) => {
  const t = useT();
  const time = useRelativeTime();
  const sizeText = useSizeText();
  const Push = useToastStore((state) => state.Push);
  const entries = provenanceEntries(file.Provenance);
  const lifetime =
    file.PinnedUntil === undefined
      ? t('ingest.file.expires', { when: time.full(file.ExpiresAt) })
      : t('ingest.file.pinned', { when: time.full(file.PinnedUntil) });

  return (
    <li className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-2 border-t border-line px-card py-3 first:border-t-0">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Icon name="file" size={18} className="flex-none text-secondary" />
        <span className="grid min-w-0 flex-1 gap-0.5">
          <span className="min-w-0 text-label">
            <bdi dir="ltr" className="font-mono break-all">
              {file.Name}
            </bdi>
          </span>
          <small className="text-body-sm text-tertiary">{[sizeText(file.SizeBytes), lifetime].join(' · ')}</small>
        </span>
        {file.ProvenanceVerified ? (
          <Badge tone="ok" icon="check">
            {t('ingest.file.verified')}
          </Badge>
        ) : (
          <Badge tone="warn" icon="alert">
            {t('ingest.file.unverified')}
          </Badge>
        )}
      </div>
      <dl className="m-0 grid gap-1 ps-8">
        <Fact label={t('ingest.file.digest')}>
          <span className="inline-flex items-center gap-1">
            <bdi dir="ltr" className="font-mono">
              {`sha256:${shortDigest(file.Digest)}`}
            </bdi>
            <IconButton
              size="sm"
              plain
              icon="copy"
              label={t('ingest.file.copyDigest', { name: file.Name })}
              onClick={() => {
                void copyText(file.Digest).then((done) => {
                  Push(
                    done
                      ? { Id: `ingest-digest-copied-${file.Id}`, Accent: 'live', Title: t('ingest.file.digestCopied') }
                      : { Id: `ingest-digest-copy-failed-${file.Id}`, Accent: 'awaiting', Title: t('projects.copy.failed') },
                  );
                });
              }}
            />
          </span>
        </Fact>
        <Fact label={t('ingest.file.ref')}>
          <bdi dir="ltr" className="font-mono">
            {shortRefName(file.Ref)}
          </bdi>{' '}
          <bdi dir="ltr" className="font-mono text-tertiary">
            {shortCommit(file.Sha)}
          </bdi>
        </Fact>
        <Fact label={t('ingest.file.uploadedBy')}>{t('ingest.file.byToken', { name: file.TokenName })}</Fact>
        <Fact label={t('ingest.file.created')}>{time.ago(file.CreatedAt)}</Fact>
        {file.RunId === undefined ? (
          <Fact label={t('ingest.file.run')}>{t('ingest.file.noRun')}</Fact>
        ) : (
          <Fact label={t('ingest.file.run')}>
            <a
              href={runHref(file.RunId)}
              className="text-primary underline"
              onClick={followInApp(() => {
                if (file.RunId !== undefined) onOpenRun(file.RunId);
              })}
            >
              {file.RunNumber === undefined ? t('ingest.file.runOpen') : t('ingest.file.runNumber', { number: file.RunNumber })}
            </a>
          </Fact>
        )}
      </dl>
      {entries.length > 0 ? (
        <details className="ps-8">
          <summary className="cursor-pointer text-body-sm text-secondary">{t('ingest.file.provenance')}</summary>
          <p className="m-0 mt-1 text-body-sm text-tertiary">{t('ingest.file.provenance.about')}</p>
          <dl className="m-0 mt-1 grid gap-1">
            {entries.map(([key, value]) => (
              <Fact key={key} label={key}>
                <bdi dir="ltr" className="font-mono break-all">
                  {value}
                </bdi>
              </Fact>
            ))}
          </dl>
        </details>
      ) : null}
    </li>
  );
};

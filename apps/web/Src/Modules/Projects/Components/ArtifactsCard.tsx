import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { Icon } from '@/Core/Ui/Icon';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { artifactDownloadHref, type TArtifact } from '@/Modules/Projects/Api/RunsApi';
import { useRunText } from '@/Modules/Projects/Hooks/useRunText';
import { useArtifacts } from '@/Modules/Projects/Hooks/useRuns';

export interface IArtifactsCardProps {
  organizationId: string;
  projectId: string;
  runId: string;
  final: boolean;
}

const ArtifactRow = ({ artifact, href }: { artifact: TArtifact; href: string }) => {
  const t = useT();
  const time = useRelativeTime();
  const text = useRunText();
  return (
    <li className="flex min-w-0 flex-wrap items-center gap-3 border-t border-line px-card py-3 first:border-t-0">
      <Icon name="file" size={18} className="text-secondary" />
      <span className="grid min-w-0 flex-1 gap-0.5">
        <span className="min-w-0 text-label">
          <bdi dir="ltr" className="break-all font-mono">
            {artifact.Name}
          </bdi>
        </span>
        <small className="text-body-sm text-tertiary">
          {[text.size(artifact.SizeBytes), t('artifacts.expires', { when: time.full(artifact.ExpiresAt) }), artifact.RunJobKey].join(' · ')}
        </small>
      </span>
      {artifact.Untrusted ? (
        <Badge tone="warn" icon="alert">
          {t('artifacts.untrusted')}
        </Badge>
      ) : null}
      <a
        href={href}
        download
        className="inline-flex h-btn-sm items-center gap-2 rounded-btn border border-line-2 px-3 text-label-sm text-primary no-underline hover:bg-surface-raised"
      >
        <Icon name="download" size={15} />
        {t('artifacts.download')}
      </a>
    </li>
  );
};

export const ArtifactsCard = ({ organizationId, projectId, runId, final }: IArtifactsCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const artifacts = useArtifacts(organizationId, projectId, runId, final);
  const untrusted = artifacts.data?.some((artifact) => artifact.Untrusted) ?? false;

  let body;
  if (artifacts.isError) {
    body = (
      <div className="grid justify-items-start gap-3">
        <InlineAlert accent="failed" title={t('artifacts.error')}>
          {errorText(artifacts.error)}
        </InlineAlert>
        <Button size="sm" onClick={() => void artifacts.refetch()}>
          {t('projects.retry')}
        </Button>
      </div>
    );
  } else if (artifacts.data === undefined) {
    body = <Skeleton label={t('artifacts.loading')} lines={2} />;
  } else if (artifacts.data.length === 0) {
    body = <p className="m-0 text-body-sm text-secondary">{final ? t('artifacts.empty') : t('artifacts.empty.running')}</p>;
  }
  return (
    <Card title={t('artifacts.title')} icon="file" padded={body !== undefined}>
      {body ?? (
        <>
          {untrusted ? <p className="m-0 px-card pb-2 text-body-sm text-secondary">{t('artifacts.untrusted.about')}</p> : null}
          <ul aria-label={t('artifacts.title')} className="m-0 grid list-none p-0">
            {(artifacts.data ?? []).map((artifact) => (
              <ArtifactRow key={artifact.Id} artifact={artifact} href={artifactDownloadHref(organizationId, projectId, artifact.Id)} />
            ))}
          </ul>
        </>
      )}
    </Card>
  );
};

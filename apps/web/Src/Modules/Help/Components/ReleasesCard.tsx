import type { IRelease } from '@/Core/Help/HelpTypes';
import { useMe } from '@/Core/Auth/useMe';
import { Card } from '@/Core/Ui/Card';
import { useI18n } from '@/I18n/useI18n';
import { ReleaseNotes } from '@/Modules/Help/Components/ReleaseNotes';

export const ReleasesCard = ({ releases }: { releases: readonly IRelease[] }) => {
  const { t } = useI18n();
  const owner = useMe().data?.InstanceOwner === true;

  return (
    <Card title={t('help.releases.title')} icon="info">
      {releases.map((release) => (
        <article key={release.Version} className="grid gap-3 border-t border-line px-card py-card first:border-t-0">
          <h4 className="m-0 text-label text-primary">
            <bdi>{release.Title}</bdi>
            <span className="ms-2 text-body-sm text-tertiary">
              <bdi>{t('help.releases.version', { version: release.Version })}</bdi>
            </span>
          </h4>
          <ReleaseNotes release={release} owner={owner} />
        </article>
      ))}
    </Card>
  );
};

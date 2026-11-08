import { useMe } from '@/Core/Auth/useMe';
import { LATEST_RELEASE } from '@/Core/Help/Generated/ReleaseMeta';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useI18n } from '@/I18n/useI18n';
import { ReleaseNotes } from '@/Modules/Help/Components/ReleaseNotes';
import { useHelpContent } from '@/Modules/Help/Hooks/useHelpContent';
import { useReleaseNotesSeen } from '@/Modules/Help/Hooks/useReleaseNotesSeen';

export interface IWhatsNewCardProps {
  onOpenAll: () => void;
}

const Notes = () => {
  const { t } = useI18n();
  const owner = useMe().data?.InstanceOwner === true;
  const content = useHelpContent();
  const release = content.data?.Releases.find((item) => item.Version === LATEST_RELEASE) ?? content.data?.Releases[0];

  if (content.isError) return null;
  if (release === undefined) return <Skeleton label={t('help.loading')} lines={3} className="px-card pb-card" />;
  return (
    <div className="px-card pb-2">
      <ReleaseNotes release={release} owner={owner} />
    </div>
  );
};

export const WhatsNewCard = ({ onOpenAll }: IWhatsNewCardProps) => {
  const { t } = useI18n();
  const seen = useReleaseNotesSeen();
  if (!seen.unseen) return null;

  return (
    <Card title={t('whatsNew.title', { version: LATEST_RELEASE })} icon="info">
      <Notes />
      {seen.failed ? (
        <div className="px-card pb-2">
          <InlineAlert accent="awaiting" title={t('whatsNew.saveFailed')} />
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2 px-card pt-2 pb-card">
        <Button variant="primary" disabled={seen.saving} onClick={seen.Dismiss}>
          {t('whatsNew.dismiss')}
        </Button>
        <Button variant="ghost" onClick={onOpenAll}>
          {t('whatsNew.all')}
        </Button>
      </div>
    </Card>
  );
};

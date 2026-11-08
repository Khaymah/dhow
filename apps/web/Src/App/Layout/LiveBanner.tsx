import { useCurrentMembership } from '@/Core/Auth/useMe';
import { useOrgEventsMode } from '@/Core/Hooks/useOrgEvents';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';

export const LiveBanner = () => {
  const t = useT();
  const mode = useOrgEventsMode(useCurrentMembership()?.OrganizationId);
  if (mode !== 'polling') return null;
  return (
    <div role="status" className="mb-4">
      <InlineAlert accent="awaiting" title={t('live.polling')} />
    </div>
  );
};

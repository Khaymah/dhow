import { useMe } from '@/Core/Auth/useMe';
import { useMembers } from '@/Core/Auth/useMembers';
import { useT } from '@/I18n/useT';

const ALL_MEMBERS = 100;

export const useCreatorName = (organizationId: string, enabled: boolean): ((userId: string | undefined) => string) => {
  const t = useT();
  const me = useMe();
  const members = useMembers(enabled ? organizationId : undefined, 1, ALL_MEMBERS);

  return (userId) => {
    if (userId === undefined) return t('ingest.creator.unknown');
    if (me.data?.User.Id === userId) return t('ingest.creator.you');
    return members.data?.Items.find((member) => member.UserId === userId)?.Name ?? t('ingest.creator.unknown');
  };
};

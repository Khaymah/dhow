import { useMutation } from '@tanstack/react-query';

import type { ApiError } from '@/Core/Api/ApiError';
import { canManage } from '@/Core/Auth/AuthTypes';
import { useCurrentMembership, useMe } from '@/Core/Auth/useMe';
import { useMembers } from '@/Core/Auth/useMembers';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Avatar } from '@/Core/Ui/Avatar';
import { Button } from '@/Core/Ui/Button';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { removeMember, type TMemberChange } from '@/Modules/Organization/Api/OrganizationApi';
import { ALL_MEMBERS, useRefreshOrganization } from '@/Modules/Organization/Hooks/useOrganization';

export interface IMemberRemoveBodyProps {
  userId: string | undefined;
  onDone: () => void;
  onCancel: () => void;
}

export const MemberRemoveBody = ({ userId, onDone, onCancel }: IMemberRemoveBodyProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const membership = useCurrentMembership();
  const me = useMe().data?.User.Id;
  const organizationId = membership?.OrganizationId ?? '';
  const members = useMembers(membership?.OrganizationId, 1, ALL_MEMBERS);
  const refresh = useRefreshOrganization(organizationId);
  const Push = useToastStore((state) => state.Push);
  const member = members.data?.Items.find((item) => item.UserId === userId);
  const remove = useMutation<TMemberChange, ApiError>({
    mutationFn: () => removeMember(organizationId, userId ?? ''),
    onSuccess: (change) => {
      Push({
        Id: `removed-${change.UserId}`,
        Accent: 'live',
        Title: t('member.remove.done', { name: member?.Name ?? '' }),
      });
      void refresh();
      onDone();
    },
  });

  if (members.isError) return <InlineAlert accent="failed" title={t('members.error')}>{errorText(members.error)}</InlineAlert>;
  if (membership === undefined || members.data === undefined) return <Skeleton label={t('ui.loading')} lines={3} />;
  if (member === undefined || member.UserId === me || member.Role === 'owner' || !canManage(membership.Role, member.Role)) {
    return <EmptyState icon="user" text={t('member.remove.unavailable')} />;
  }

  return (
    <div className="grid gap-4">
      <div className="flex min-w-0 items-center gap-3 rounded-md border border-line p-card">
        <Avatar name={member.Name} />
        <div className="grid min-w-0 gap-0.5">
          <bdi className="truncate text-label">{member.Name}</bdi>
          <bdi dir="ltr" className="truncate text-start text-body-sm text-secondary">
            {member.Email}
          </bdi>
        </div>
      </div>
      <p className="m-0 text-body text-secondary">{t('member.remove.question', { organization: membership.OrganizationName })}</p>
      <p className="m-0 text-body-sm text-tertiary">{t('member.remove.about', { name: member.Name })}</p>
      {remove.error ? <ErrorAlert error={remove.error} /> : null}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="danger"
          iconStart="trash"
          disabled={remove.isPending}
          onClick={() => {
            remove.mutate();
          }}
        >
          {remove.isPending ? t('member.remove.removing') : t('member.remove.confirm')}
        </Button>
        <Button variant="ghost" disabled={remove.isPending} onClick={onCancel}>
          {t('member.cancel')}
        </Button>
      </div>
    </div>
  );
};

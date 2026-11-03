import { useNavigate, useRouter } from '@tanstack/react-router';

import { useCurrentMembership } from '@/Core/Auth/useMe';
import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import { useT } from '@/I18n/useT';

export interface IOrganizationChipProps {
  className?: string;
}

export const OrganizationChip = ({ className }: IOrganizationChipProps) => {
  const t = useT();
  const navigate = useNavigate();
  const router = useRouter();
  const membership = useCurrentMembership();
  if (membership === undefined) return null;
  const preloadOrganization = () => {
    void router.preloadRoute({ to: '/organizations' }).catch(() => undefined);
  };

  return (
    <button
      type="button"
      onPointerEnter={preloadOrganization}
      onFocus={preloadOrganization}
      aria-label={t('shell.organization.switch', { name: membership.OrganizationName })}
      onClick={() => {
        void navigate({ to: '/organizations' });
      }}
      className={cx(
        'flex min-w-0 max-w-full cursor-pointer items-center gap-2 rounded-sm border border-line bg-transparent px-2.5 py-1.5',
        'text-start text-body-sm text-secondary transition-colors duration-140 hover:bg-hover hover:text-primary',
        className,
      )}
    >
      <Icon name="building" size={16} />
      <span dir="auto" className="truncate">{membership.OrganizationName}</span>
      <Icon name="swap" size={14} className="ms-auto flex-none text-tertiary" />
    </button>
  );
};

import { useNavigate } from '@tanstack/react-router';

import { sectionById, type TSectionId } from '@/App/Navigation/Sections';
import { useSection } from '@/App/Navigation/useSection';
import { useCurrentMembership, useMe } from '@/Core/Auth/useMe';
import { SectionNav, type ISectionNavItem } from '@/Core/Ui/SectionNav';
import type { TMessageKey } from '@/I18n/Messages';
import { useT } from '@/I18n/useT';

interface ISectionNavsProps {
  label: TMessageKey;
  ids: readonly TSectionId[];
  className?: string;
}

const SectionLinks = ({ label, ids, className }: ISectionNavsProps) => {
  const t = useT();
  const navigate = useNavigate();
  const match = useSection();
  const role = useCurrentMembership()?.Role;
  const owner = useMe().data?.InstanceOwner === true;
  const items: ISectionNavItem[] = ids
    .map(sectionById)
    .filter((section) => section.allow(role, owner))
    .map((section) => ({
      id: section.id,
      label: t(section.label),
      href: section.path,
      current: match?.section.id === section.id,
      onSelect: () => {
        void navigate({ to: section.path });
      },
    }));
  if (items.length < 2) return null;
  return <SectionNav label={t(label)} items={items} className={className} />;
};

const ORG_IDS: readonly TSectionId[] = ['members', 'teams', 'invitations', 'apiKeys', 'secrets', 'orgSettings'];
const ACCOUNT_IDS: readonly TSectionId[] = ['profile', 'security', 'preferences'];
const RUNNER_IDS: readonly TSectionId[] = ['runners', 'runnerTokens'];

export const OrgNav = () => <SectionLinks label="shell.sections.org" ids={ORG_IDS} className="md:hidden" />;

export const AccountNav = () => <SectionLinks label="shell.sections.account" ids={ACCOUNT_IDS} />;

export const RunnersNav = () => <SectionLinks label="shell.sections.runners" ids={RUNNER_IDS} />;

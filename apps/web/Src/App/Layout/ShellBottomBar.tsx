import { useNavigate } from '@tanstack/react-router';

import { member } from '@/App/Navigation/Gates';
import { visibleSections, type TSectionPath } from '@/App/Navigation/Sections';
import { useCounters } from '@/App/Navigation/useCounters';
import { useSection } from '@/App/Navigation/useSection';
import { canReadRunners } from '@/Core/Auth/AuthTypes';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { BottomBar, type IBottomBarItem } from '@/Core/Ui/BottomBar';
import type { TIconName } from '@/Core/Ui/Icons';
import { useT } from '@/I18n/useT';

export const ShellBottomBar = () => {
  const t = useT();
  const navigate = useNavigate();
  const match = useSection();
  const role = useCurrentMembership()?.Role;
  const section = match?.section;
  const { jobsDot } = useCounters();
  const orgPath = visibleSections(role, 'org').find((entry) => entry.menu)?.path ?? '/organizations';

  const entry = (id: string, icon: TIconName, label: string, path: TSectionPath, current: boolean): IBottomBarItem => ({
    id,
    icon,
    label,
    href: path,
    current,
    onSelect: () => {
      void navigate({ to: path });
    },
  });

  const items: IBottomBarItem[] = [
    entry('home', 'home', t('shell.nav.home'), '/', section?.id === 'home'),
    ...(member(role) ? [entry('projects', 'folder', t('shell.nav.projects'), '/projects', section?.id === 'projects')] : []),
    ...(member(role) ? [{ ...entry('jobs', 'play', t('shell.nav.jobs'), '/jobs', section?.id === 'jobs'), dot: jobsDot }] : []),
    ...(canReadRunners(role)
      ? [entry('runners', 'server', t('shell.bottom.runners'), '/runners', section?.id === 'runners' || section?.parent === 'runners')]
      : []),
    entry('org', 'users', t('shell.bottom.org'), orgPath, section?.group === 'org'),
    entry('account', 'user', t('shell.group.account'), '/account/profile', section?.group === 'account'),
  ];

  return <BottomBar label={t('shell.bottom.label')} items={items} />;
};

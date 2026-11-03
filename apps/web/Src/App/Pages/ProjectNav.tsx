import { useNavigate } from '@tanstack/react-router';

import { SectionNav } from '@/Core/Ui/SectionNav';
import { useT } from '@/I18n/useT';

export type TProjectTab = 'runs' | 'settings';

export const ProjectNav = ({ projectId, current }: { projectId: string; current: TProjectTab }) => {
  const t = useT();
  const navigate = useNavigate();
  return (
    <SectionNav
      label={t('projects.nav.label')}
      items={[
        {
          id: 'runs',
          label: t('projects.nav.runs'),
          href: `/projects/${projectId}`,
          current: current === 'runs',
          onSelect: () => {
            void navigate({ to: '/projects/$projectId', params: { projectId } });
          },
        },
        {
          id: 'settings',
          label: t('projects.nav.settings'),
          href: `/projects/${projectId}/settings`,
          current: current === 'settings',
          onSelect: () => {
            void navigate({ to: '/projects/$projectId/settings', params: { projectId } });
          },
        },
      ]}
    />
  );
};

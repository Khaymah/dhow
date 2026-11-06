import { useNavigate } from '@tanstack/react-router';

import { canReadSecrets, type TRole } from '@/Core/Auth/AuthTypes';
import { SectionNav } from '@/Core/Ui/SectionNav';
import { useT } from '@/I18n/useT';
import { canReadIngested } from '@/Modules/Ingest/Types/IngestAccess';

export type TProjectTab = 'runs' | 'environments' | 'ingest' | 'secrets' | 'settings';

export const ProjectNav = ({ projectId, current, role }: { projectId: string; current: TProjectTab; role?: TRole }) => {
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
          id: 'environments',
          label: t('projects.nav.environments'),
          href: `/projects/${projectId}/environments`,
          current: current === 'environments',
          onSelect: () => {
            void navigate({ to: '/projects/$projectId/environments', params: { projectId } });
          },
        },
        ...(canReadIngested(role)
          ? [
              {
                id: 'ingest',
                label: t('projects.nav.ingest'),
                href: `/projects/${projectId}/ingest`,
                current: current === 'ingest',
                onSelect: () => {
                  void navigate({ to: '/projects/$projectId/ingest', params: { projectId } });
                },
              },
            ]
          : []),
        ...(canReadSecrets(role)
          ? [
              {
                id: 'secrets',
                label: t('projects.nav.secrets'),
                href: `/projects/${projectId}/secrets`,
                current: current === 'secrets',
                onSelect: () => {
                  void navigate({ to: '/projects/$projectId/secrets', params: { projectId } });
                },
              },
            ]
          : []),
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

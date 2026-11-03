import { useNavigate } from '@tanstack/react-router';

import { Page } from '@/App/Layout/Page';
import { useSlide } from '@/App/Navigation/useSlide';
import { useT } from '@/I18n/useT';
import { ProjectsBody, ProjectsFooter } from '@/Modules/Projects/Components/ProjectsBody';

export const ProjectsPage = () => {
  const t = useT();
  const navigate = useNavigate();
  const { open } = useSlide();
  const HandleCreate = () => {
    open('project-new');
  };

  return (
    <Page title={t('projects.title')} lead={t('projects.subtitle')} action={<ProjectsFooter onCreate={HandleCreate} />}>
      <ProjectsBody
        hrefFor={(projectId) => `/projects/${projectId}`}
        onOpenProject={(projectId) => {
          void navigate({ to: '/projects/$projectId', params: { projectId } });
        }}
        onCreate={HandleCreate}
      />
    </Page>
  );
};

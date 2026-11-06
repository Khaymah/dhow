import { useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams } from '@tanstack/react-router';
import { useEffect, useEffectEvent, useState } from 'react';

import { buildSlide } from '@/App/Layout/SlideCatalog';
import { useSlide } from '@/App/Navigation/useSlide';
import { isEnvironmentName, isUuid } from '@/App/Pages/Params';
import { isProjectSlide, isThemeEditSlide, slideAllowed, slideScopeOf, themeOf, type TSlideId } from '@/App/Panels';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import type { ICustomTheme } from '@/Core/Theme/ThemeTokens';
import { useThemeStore } from '@/Core/Theme/useThemeStore';
import { useSlideGuardStore } from '@/Core/Store/useSlideGuardStore';
import { useToastStore } from '@/Core/Store/useToastStore';
import { SlideStack } from '@/Core/Ui/SlideStack';
import { OVERVIEW_QUERY_KEY } from '@/Modules/Overview/Hooks/useOverview';
import { runsKey } from '@/Modules/Projects/Api/RunsApi';
import { useProject } from '@/Modules/Projects/Hooks/useProjects';
import { useT } from '@/I18n/useT';

const themeMissing = (slide: TSlideId | null, themes: readonly ICustomTheme[]): boolean => {
  const themeId = slide === null ? null : themeOf(slide);
  return themeId !== null && !themes.some((theme) => theme.Id === themeId);
};

export const SlideHost = () => {
  const t = useT();
  const navigate = useNavigate();
  const { slide, invalid, close, discard } = useSlide();
  const { memberId, projectId, environmentName, runId } = useParams({ strict: false });
  const membership = useCurrentMembership();
  const role = membership?.Role;
  const client = useQueryClient();
  const draftName = useThemeStore((state) => state.Draft?.Name);
  const customThemes = useThemeStore((state) => state.CustomThemes);
  const DiscardDraft = useThemeStore((state) => state.DiscardDraft);
  const Push = useToastStore((state) => state.Push);
  const Blocked = useSlideGuardStore((state) => state.Blocked);
  const Attempt = useSlideGuardStore((state) => state.Attempt);
  const [Seen, setSeen] = useState<TSlideId | null>(null);
  const [Missing, setMissing] = useState(false);
  if (slide !== Seen) {
    setSeen(slide);
    setMissing(themeMissing(slide, customThemes));
  }
  const badEnvironment = environmentName !== undefined && !isEnvironmentName(environmentName);
  const projectSlide = slide !== null && isProjectSlide(slide) && isUuid(projectId);
  const project = useProject(projectSlide && membership !== undefined ? membership.OrganizationId : '', projectSlide ? projectId : '');
  const projectPending = projectSlide && project.data === undefined && !project.isError;
  const projectRole = project.data?.CallerRole;
  const projectActive = project.isError ? false : project.data === undefined ? undefined : project.data.State === 'active';
  const scope = slideScopeOf(isUuid(projectId), isEnvironmentName(environmentName));
  const refused = slide !== null && (badEnvironment || (!projectPending && !slideAllowed(slide, role, projectRole, scope)));
  const editing = slide !== null && isThemeEditSlide(slide);

  const rejectLink = useEffectEvent(() => {
    Push({
      Id: 'unknown-panel',
      Accent: 'awaiting',
      Title: t('toast.unknownPanel.title'),
      Detail: t('toast.unknownPanel.detail'),
    });
    discard();
  });

  useEffect(() => {
    if (!editing) DiscardDraft();
  }, [editing, DiscardDraft]);

  useEffect(() => {
    if (invalid || Missing || refused) rejectLink();
  }, [invalid, Missing, refused]);

  const slides =
    slide === null || refused || Missing || projectPending
      ? []
      : [
          buildSlide(slide, {
            t,
            close,
            draftName,
            customThemes,
            openTeam: (teamId) => {
              void navigate({ to: '/org/teams/$teamId', params: { teamId } });
            },
            openOrganization: () => {
              void navigate({ to: '/org/members' });
            },
            memberId: isUuid(memberId) ? memberId : undefined,
            openMembers: () => {
              void navigate({ to: '/org/members' });
            },
            openHome: () => {
              void navigate({ to: '/' });
            },
            projectId: isUuid(projectId) ? projectId : undefined,
            projectRole,
            projectActive,
            openProject: (id) => {
              void navigate({ to: '/projects/$projectId', params: { projectId: id } });
            },
            environmentName: isEnvironmentName(environmentName) ? environmentName : undefined,
            openEnvironment: (id, name) => {
              void navigate({ to: '/projects/$projectId/environments/$environmentName', params: { projectId: id, environmentName: name } });
            },
            openRun: (id, openedRunId) => {
              void navigate({ to: '/projects/$projectId/runs/$runId', params: { projectId: id, runId: openedRunId } });
            },
            runId: isUuid(runId) ? runId : undefined,
            decided: () => {
              if (membership === undefined || !isUuid(projectId)) return;
              void client.invalidateQueries({ queryKey: runsKey(membership.OrganizationId, projectId) });
              void client.invalidateQueries({ queryKey: OVERVIEW_QUERY_KEY });
            },
            openSecurity: () => {
              void navigate({ to: '/account/security' });
            },
          }),
        ];

  return <SlideStack slides={slides} onPop={close} onCloseAll={close} onJump={close} closeGuard={{ blocked: Blocked, onBlocked: Attempt }} />;
};

import { useNavigate, useParams } from '@tanstack/react-router';
import { useEffect, useState } from 'react';

import type { ISettingsCommands } from '@/App/Layout/SettingsCommands';
import { isUuid } from '@/App/Pages/Params';
import type { ISettingsContext } from '@/App/Navigation/SettingsRegistry';
import { useCurrentMembership, useMe } from '@/Core/Auth/useMe';
import { usePaletteStore } from '@/Core/Store/usePaletteStore';
import type { ICommandItem } from '@/Core/Ui/CommandPalette';
import { useT } from '@/I18n/useT';
import { useProject } from '@/Modules/Projects/Hooks/useProjects';

const PREFIX = 'setting:';

export interface ISettingCommands {
  items: ICommandItem[];
  owns: (item: ICommandItem) => boolean;
  run: (item: ICommandItem) => void;
}

type TLoaded = typeof import('@/App/Layout/SettingsCommands');

export const useSettingsCommands = (): ISettingCommands => {
  const t = useT();
  const navigate = useNavigate();
  const membership = useCurrentMembership();
  const owner = useMe().data?.InstanceOwner === true;
  const params = useParams({ strict: false });
  const paletteOpen = usePaletteStore((state) => state.Open);
  const [Loaded, setLoaded] = useState<TLoaded | null>(null);
  const project = useProject(membership?.OrganizationId ?? '', isUuid(params.projectId) ? params.projectId : '');

  useEffect(() => {
    if (paletteOpen && Loaded === null) void import('@/App/Layout/SettingsCommands').then(setLoaded);
  }, [paletteOpen, Loaded]);

  if (Loaded === null) return { items: [], owns: () => false, run: () => undefined };
  const context: ISettingsContext = { role: membership?.Role, owner, projectActive: true };
  const projectContext: ISettingsContext = { role: project.data?.CallerRole ?? membership?.Role, owner, projectActive: project.data?.State === 'active' };
  const built: ISettingsCommands = Loaded.settingCommands({
    t,
    context,
    projectContext,
    projectId: project.data === undefined ? null : (params.projectId ?? null),
    navigate,
  });
  return { items: built.items, owns: (item) => item.id.startsWith(PREFIX), run: built.run };
};

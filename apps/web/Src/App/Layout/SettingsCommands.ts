import {
  SETTINGS,
  SETTINGS_PAGES,
  settingVisible,
  type ISettingEntry,
  type ISettingsContext,
  type TSettingsPage,
} from '@/App/Navigation/SettingsRegistry';
import type { ICommandItem } from '@/Core/Ui/CommandPalette';
import type { useT } from '@/I18n/useT';

export const SETTING_ITEM_PREFIX = 'setting:';

type TAccountPath = '/org/settings' | '/system/settings' | '/account/security' | '/account/preferences';

const PAGE_PATHS: Partial<Record<TSettingsPage, TAccountPath>> = {
  org: '/org/settings',
  system: '/system/settings',
  security: '/account/security',
  preferences: '/account/preferences',
};

export type TSettingsNavigate = (target: { to: string; params?: { projectId: string }; hash?: string }) => Promise<void>;

export interface ISettingsCommandInput {
  t: ReturnType<typeof useT>;
  context: ISettingsContext;
  projectContext: ISettingsContext;
  projectId: string | null;
  navigate: TSettingsNavigate;
}

export interface ISettingsCommands {
  items: ICommandItem[];
  run: (item: ICommandItem) => void;
}

export const settingCommands = (input: ISettingsCommandInput): ISettingsCommands => {
  const { projectId } = input;
  const available = SETTINGS.filter((entry) => {
    if (SETTINGS_PAGES[entry.page].scoped) return projectId !== null && settingVisible(entry, input.projectContext);
    return settingVisible(entry, input.context);
  });

  const labelOf = (entry: ISettingEntry): string => {
    const page = input.t(SETTINGS_PAGES[entry.page].label);
    return entry.anchor === null ? page : input.t('palette.setting', { page, section: input.t(entry.label) });
  };

  const items = available.map((entry) => ({
    id: `${SETTING_ITEM_PREFIX}${entry.id}`,
    icon: entry.icon,
    label: labelOf(entry),
    kind: input.t('palette.kind.setting'),
  }));

  const run = (item: ICommandItem) => {
    const entry = available.find((candidate) => `${SETTING_ITEM_PREFIX}${candidate.id}` === item.id);
    if (entry === undefined) return;
    const hash = entry.anchor ?? undefined;
    const fixed = PAGE_PATHS[entry.page];
    if (fixed !== undefined) void input.navigate({ to: fixed, hash });
    else if (projectId !== null) {
      const to = entry.page === 'project' ? '/projects/$projectId/settings' : '/projects/$projectId/notifications';
      void input.navigate({ to, params: { projectId }, hash });
    }
  };

  return { items, run };
};

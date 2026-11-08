import type { ReactNode } from 'react';

import { AnchorTarget } from '@/App/Layout/AnchorTarget';
import { PageGroup } from '@/App/Layout/PageGroup';
import { settingEntry, type TSettingId } from '@/App/Navigation/SettingsRegistry';
import type { TIconName } from '@/Core/Ui/Icons';
import { useT } from '@/I18n/useT';

export interface ISettingsGroupProps {
  setting: TSettingId;
  lead?: string;
  icon?: TIconName;
  danger?: boolean;
  children: ReactNode;
}

export const SettingsGroup = ({ setting, lead, icon, danger, children }: ISettingsGroupProps) => {
  const t = useT();
  const entry = settingEntry(setting);
  const group = (
    <PageGroup title={t(entry.label)} lead={lead} icon={icon ?? entry.icon} danger={danger}>
      {children}
    </PageGroup>
  );
  return entry.anchor === null ? group : <AnchorTarget anchor={entry.anchor}>{group}</AnchorTarget>;
};

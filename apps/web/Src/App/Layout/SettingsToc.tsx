import { useNavigate } from '@tanstack/react-router';

import { settingsOfPage, settingVisible, type ISettingsContext, type TSettingsPage } from '@/App/Navigation/SettingsRegistry';
import { followInApp } from '@/Core/Ui/PlainClick';
import { useT } from '@/I18n/useT';

export interface ISettingsTocProps {
  page: TSettingsPage;
  context: ISettingsContext;
}

export const SettingsToc = ({ page, context }: ISettingsTocProps) => {
  const t = useT();
  const navigate = useNavigate();
  const entries = settingsOfPage(page).filter((entry) => entry.anchor !== null && settingVisible(entry, context));
  if (entries.length < 2) return null;

  return (
    <nav aria-label={t('settings.toc')} className="min-w-0">
      <ul className="m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-body-sm">
        {entries.map((entry) => (
          <li key={entry.id}>
            <a
              href={`#${entry.anchor ?? ''}`}
              onClick={followInApp(() => {
                void navigate({ to: '.', hash: entry.anchor ?? undefined });
              })}
              className="inline-flex min-h-6 items-center text-secondary underline-offset-4 hover:text-primary hover:underline"
            >
              {t(entry.label)}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
};

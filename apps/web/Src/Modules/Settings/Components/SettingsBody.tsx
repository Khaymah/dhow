import type { ReactNode } from 'react';

import { cardStateOf } from '@/Core/Ui/CardStates';
import { useT } from '@/I18n/useT';
import { MailTest } from '@/Modules/Settings/Components/MailTest';
import { OidcCard } from '@/Modules/Settings/Components/OidcCard';
import { SettingsCard } from '@/Modules/Settings/Components/SettingsCard';
import { useInstanceSettings } from '@/Modules/Settings/Hooks/useInstanceSettings';
import { SETTINGS_GROUPS } from '@/Modules/Settings/Types/SettingsGroups';

export const PROVIDERS_ANCHOR = 'providers';

const plain = (_anchor: string, node: ReactNode): ReactNode => node;

export interface ISettingsBodyProps {
  section?: (anchor: string, node: ReactNode) => ReactNode;
  onAddProvider: () => void;
  onEditProvider: (id: string) => void;
}

export const SettingsBody = ({ section = plain, onAddProvider, onEditProvider }: ISettingsBodyProps) => {
  const t = useT();
  const query = useInstanceSettings();
  const state = cardStateOf(query, false);
  const mailOn = query.data !== undefined && query.data.Mail.Host.Value !== '';

  return (
    <div className="grid gap-block">
      {SETTINGS_GROUPS.map((spec) => [
        <div key={spec.Key} className="min-w-0">
          {section(
            spec.Path,
            <SettingsCard
              spec={spec}
              settings={query.data}
              state={state}
              onRetry={() => {
                void query.refetch();
              }}
              emptyNote={spec.Key === 'Mail' ? t('settings.mail.off') : t('settings.allDefault')}
              footer={spec.Key === 'Mail' ? <MailTest enabled={mailOn} /> : undefined}
            />,
          )}
        </div>,
        spec.Key === 'Mail' ? (
          <div key="oidc" className="min-w-0">
            {section(PROVIDERS_ANCHOR, <OidcCard onAdd={onAddProvider} onEdit={onEditProvider} />)}
          </div>
        ) : null,
      ])}
    </div>
  );
};

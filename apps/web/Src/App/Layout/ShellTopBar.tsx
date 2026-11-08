import { useNavigate } from '@tanstack/react-router';

import { OrganizationChip } from '@/App/Layout/OrganizationChip';
import { ServerStatusLink } from '@/App/Layout/ServerStatusLink';
import { ShellCrumbs } from '@/App/Layout/ShellCrumbs';
import { shortcutLabel } from '@/App/Layout/ShortcutLabel';
import { usePaletteStore } from '@/Core/Store/usePaletteStore';
import { Button } from '@/Core/Ui/Button';
import { IconButton } from '@/Core/Ui/IconButton';
import { TopBar } from '@/Core/Ui/TopBar';
import { otherLocale } from '@/I18n/Locale';
import { useUnreadCount } from '@/Modules/Notifications/Hooks/useUnreadCount';
import { useI18n } from '@/I18n/useI18n';

export const ShellTopBar = () => {
  const { Locale, SetLocale, t } = useI18n();
  const navigate = useNavigate();
  const SetPaletteOpen = usePaletteStore((state) => state.SetOpen);
  const target = otherLocale(Locale);
  const unread = useUnreadCount(true);

  return (
    <TopBar
      start={
        <>
          <ShellCrumbs className="min-w-0 shrink max-md:hidden" />
          <OrganizationChip className="md:hidden" />
        </>
      }
      searchLabel={t('shell.search')}
      shortcut={shortcutLabel()}
      onSearch={() => {
        SetPaletteOpen(true);
      }}
    >
      <ServerStatusLink />
      <IconButton
        icon="help"
        label={t('shell.nav.help')}
        onClick={() => {
          void navigate({ to: '/help' });
        }}
      />
      <IconButton
        icon="bell"
        label={unread > 0 ? t('shell.notifications.unread', { count: unread }) : t('shell.notifications')}
        count={unread}
        onClick={() => {
          void navigate({ to: '/notifications' });
        }}
      />
      <Button
        aria-label={t('shell.language.switch', { language: t(`language.${target}`) })}
        onClick={() => {
          SetLocale(target);
        }}
      >
        <span lang={target}>{t(`language.${target}`)}</span>
      </Button>
    </TopBar>
  );
};

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import { I18nContext, type II18nValue } from '@/I18n/I18nContext';
import { useToastStore } from '@/Core/Store/useToastStore';
import { DEFAULT_LOCALE, DIRECTION, readStoredLocale, storeLocale, type TLocale } from '@/I18n/Locale';
import { BootAr } from '@/I18n/Locales/Boot/Ar';
import { loadBootMessages } from '@/I18n/BootCatalog';
import { translate } from '@/I18n/Translate';
import { useCatalogStore } from '@/I18n/useCatalogStore';
import { anyWorkspaceMessages, loadWorkspaceMessages } from '@/I18n/WorkspaceCatalog';

const startLocale = (): TLocale => {
  const stored = readStoredLocale();
  return stored === DEFAULT_LOCALE || useCatalogStore.getState().Boot[stored] !== undefined ? stored : DEFAULT_LOCALE;
};

export const I18nProvider = ({ children }: { children: ReactNode }) => {
  const [Locale, setLocaleState] = useState<TLocale>(startLocale);
  const Boot = useCatalogStore((state) => state.Boot[Locale]);
  const Workspace = useCatalogStore((state) => state.Workspace[Locale]);
  const Requested = useRef<TLocale>(Locale);
  const Applied = useRef<TLocale>(Locale);

  const t = useCallback<II18nValue['t']>(
    (key, params) => translate(Locale, key, params, Workspace, Boot ?? BootAr),
    [Locale, Boot, Workspace],
  );

  const SetLocale = useCallback((next: TLocale) => {
    const previous = Applied.current;
    Requested.current = next;
    const loading = [loadBootMessages(next)];
    if (anyWorkspaceMessages()) loading.push(loadWorkspaceMessages(next));
    void Promise.all(loading).then(
      () => {
        if (Requested.current !== next) return;
        storeLocale(next);
        setLocaleState(next);
      },
      () => {
        if (Requested.current !== next) return;
        Requested.current = previous;
        useToastStore.getState().Push({
          Id: 'language-switch-failed',
          Accent: 'failed',
          Title: translate(previous, 'i18n.switchFailed'),
        });
      },
    );
  }, []);

  useEffect(() => {
    const root = document.documentElement;
    Applied.current = Locale;
    root.lang = Locale;
    root.dir = DIRECTION[Locale];
    document.title = translate(Locale, 'app.title');
  }, [Locale]);

  const value = useMemo<II18nValue>(() => ({ Locale, SetLocale, t }), [Locale, SetLocale, t]);

  return <I18nContext value={value}>{children}</I18nContext>;
};

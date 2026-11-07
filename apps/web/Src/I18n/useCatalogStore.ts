import { create } from 'zustand';

import type { TLocale } from '@/I18n/Locale';
import type { TBootMessages, TWorkspaceMessages } from '@/I18n/Messages';

interface ICatalogState {
  Boot: Partial<Record<TLocale, TBootMessages>>;
  Workspace: Partial<Record<TLocale, TWorkspaceMessages>>;
}

export const useCatalogStore = create<ICatalogState>(() => ({ Boot: {}, Workspace: {} }));

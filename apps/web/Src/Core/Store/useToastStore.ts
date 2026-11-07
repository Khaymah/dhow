import { create } from 'zustand';

import type { TAccent } from '@/Core/Ui/Accent';

export interface IToast {
  Id: string;
  Accent: TAccent;
  Title: string;
  Detail?: string;
  Fix?: string;
  HelpHref?: string | null;
}

interface IToastState {
  Toasts: IToast[];
  Push: (toast: IToast) => void;
  Dismiss: (id: string) => void;
}

export const useToastStore = create<IToastState>((set) => ({
  Toasts: [],
  Push: (toast) => {
    set((state) => ({
      Toasts: state.Toasts.some((t) => t.Id === toast.Id) ? state.Toasts : [...state.Toasts, toast],
    }));
  },
  Dismiss: (id) => {
    set((state) => ({ Toasts: state.Toasts.filter((t) => t.Id !== id) }));
  },
}));

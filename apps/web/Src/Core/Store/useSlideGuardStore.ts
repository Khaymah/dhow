import { create } from 'zustand';

interface ISlideGuardState {
  Blocked: boolean;
  Attempts: number;
  Block: (blocked: boolean) => void;
  Attempt: () => void;
}

export const useSlideGuardStore = create<ISlideGuardState>((set) => ({
  Blocked: false,
  Attempts: 0,
  Block: (blocked) => {
    set((state) => (state.Blocked === blocked ? state : { Blocked: blocked }));
  },
  Attempt: () => {
    set((state) => ({ Attempts: state.Attempts + 1 }));
  },
}));

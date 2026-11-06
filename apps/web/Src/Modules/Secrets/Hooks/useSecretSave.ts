import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { ALL_SECRETS_KEY } from '@/Modules/Secrets/Api/SecretsApi';

export interface ISecretSave {
  Busy: boolean;
  Notice: string | null;
  SetNotice: (notice: string | null) => void;
  Save: (task: () => Promise<unknown>, saved: string, clear: () => void, handled?: (error: unknown) => boolean) => void;
  Succeed: (saved: string, clear: () => void) => Promise<void>;
}

export const useSecretSave = (onDone: () => void): ISecretSave => {
  const errorText = useApiErrorText();
  const client = useQueryClient();
  const Push = useToastStore((state) => state.Push);
  const [Busy, setBusy] = useState(false);
  const [Notice, setNotice] = useState<string | null>(null);

  const Succeed = async (saved: string, clear: () => void) => {
    clear();
    await client.invalidateQueries({ queryKey: ALL_SECRETS_KEY });
    Push({
      Id: `secrets-saved-${Date.now()}`,
      Accent: 'live',
      Title: saved,
    });
    onDone();
  };

  return {
    Busy,
    Notice,
    SetNotice: setNotice,
    Succeed,
    Save: (task, saved, clear, handled) => {
      setBusy(true);
      setNotice(null);
      task()
        .then(() => Succeed(saved, clear))
        .catch((error: unknown) => {
          if (handled?.(error) !== true) setNotice(errorText(error));
        })
        .finally(() => {
          setBusy(false);
        });
    },
  };
};

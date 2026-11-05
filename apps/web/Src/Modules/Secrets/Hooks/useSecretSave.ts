import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { ALL_SECRETS_KEY } from '@/Modules/Secrets/Api/SecretsApi';

export interface ISecretSave {
  Busy: boolean;
  Notice: string | null;
  SetNotice: (notice: string | null) => void;
  Save: (task: () => Promise<unknown>, saved: string, clear: () => void) => void;
}

export const useSecretSave = (onDone: () => void): ISecretSave => {
  const errorText = useApiErrorText();
  const client = useQueryClient();
  const Push = useToastStore((state) => state.Push);
  const [Busy, setBusy] = useState(false);
  const [Notice, setNotice] = useState<string | null>(null);

  return {
    Busy,
    Notice,
    SetNotice: setNotice,
    Save: (task, saved, clear) => {
      setBusy(true);
      setNotice(null);
      task()
        .then(async () => {
          clear();
          await client.invalidateQueries({ queryKey: ALL_SECRETS_KEY });
          Push({
            Id: `secrets-saved-${Date.now()}`,
            Accent: 'live',
            Title: saved,
          });
          onDone();
        })
        .catch((error: unknown) => {
          setNotice(errorText(error));
        })
        .finally(() => {
          setBusy(false);
        });
    },
  };
};

import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { ALL_SECRETS_KEY } from '@/Modules/Secrets/Api/SecretsApi';

export interface IRemoval {
  Open: string | null;
  Busy: boolean;
  Notice: string | null;
  Ask: (key: string) => void;
  Cancel: () => void;
  Confirm: (key: string, run: () => Promise<unknown>, done: string) => void;
}

export const useRemoval = (): IRemoval => {
  const errorText = useApiErrorText();
  const client = useQueryClient();
  const Push = useToastStore((state) => state.Push);
  const [Open, setOpen] = useState<string | null>(null);
  const [Busy, setBusy] = useState(false);
  const [Notice, setNotice] = useState<string | null>(null);

  return {
    Open,
    Busy,
    Notice,
    Ask: (key) => {
      setNotice(null);
      setOpen(key);
    },
    Cancel: () => {
      setOpen(null);
    },
    Confirm: (key, run, done) => {
      setBusy(true);
      setNotice(null);
      run()
        .then(async () => {
          setOpen(null);
          await client.invalidateQueries({ queryKey: ALL_SECRETS_KEY });
          Push({ Id: `secrets-removed-${key}`, Accent: 'live', Title: done });
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

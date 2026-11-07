import { useCallback, useEffect, useRef, useState } from 'react';

import { copyText } from '@/Core/Ui/Clipboard';

export const NOTICE_MS = 3000;

export type TNotice = 'copied' | 'failed' | null;

export interface INotice {
  Notice: TNotice;
  Copy: (text: string) => void;
}

export const useNotice = (): INotice => {
  const [Notice, setNotice] = useState<TNotice>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current !== null) clearTimeout(timer.current);
    },
    [],
  );

  const Copy = useCallback((text: string) => {
    void copyText(text).then((ok) => {
      setNotice(ok ? 'copied' : 'failed');
      if (timer.current !== null) clearTimeout(timer.current);
      timer.current = setTimeout(() => {
        setNotice(null);
      }, NOTICE_MS);
    });
  }, []);

  return { Notice, Copy };
};

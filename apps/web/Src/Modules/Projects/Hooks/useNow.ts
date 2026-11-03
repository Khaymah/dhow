import { useEffect, useState } from 'react';

export const useNow = (active: boolean, every = 1000): number => {
  const [Now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!active) return undefined;
    const timer = setInterval(() => {
      setNow(Date.now());
    }, every);
    return () => {
      clearInterval(timer);
    };
  }, [active, every]);

  return Now;
};

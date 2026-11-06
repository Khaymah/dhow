import { useEffect, type RefObject } from 'react';

export const useClearOnUnmount = (ref: RefObject<HTMLTextAreaElement | null>): void => {
  useEffect(() => {
    const field = ref.current;
    return () => {
      if (field) field.value = '';
    };
  }, [ref]);
};

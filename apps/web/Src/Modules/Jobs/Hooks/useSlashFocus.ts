import { useEffect, type RefObject } from 'react';

const isTyping = (target: EventTarget | null): boolean =>
  target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement;

export const useSlashFocus = (scope: RefObject<HTMLElement | null>, input: RefObject<HTMLInputElement | null>) => {
  useEffect(() => {
    const element = scope.current;
    if (element === null) return undefined;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey || isTyping(event.target)) return;
      event.preventDefault();
      input.current?.focus();
    };
    element.addEventListener('keydown', onKeyDown);
    return () => {
      element.removeEventListener('keydown', onKeyDown);
    };
  }, [scope, input]);
};

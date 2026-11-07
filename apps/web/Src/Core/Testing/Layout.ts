export const VIEWPORT_PX = 400;
export const ROW_PX = 20;

const SIZES: Record<string, (element: HTMLElement) => number> = {
  clientHeight: (element) => (element.getAttribute('role') === 'log' ? VIEWPORT_PX : 0),
  clientWidth: (element) => (element.getAttribute('role') === 'log' ? 600 : 0),
  scrollHeight: (element) => {
    const inner = element.firstElementChild;
    return element.getAttribute('role') === 'log' && inner instanceof HTMLElement ? Number.parseFloat(inner.style.height) || 0 : 0;
  },
  offsetHeight: (element) => (element.hasAttribute('data-index') ? ROW_PX : 0),
  offsetWidth: (element) => (element.hasAttribute('data-index') ? 600 : 0),
};

export const installLayout = (): (() => void) => {
  const originals = Object.entries(SIZES).map(([name, read]) => {
    const original = Object.getOwnPropertyDescriptor(HTMLElement.prototype, name);
    Object.defineProperty(HTMLElement.prototype, name, {
      configurable: true,
      get(this: HTMLElement) {
        return read(this);
      },
    });
    return { name, original };
  });
  const scrollTo = Object.getOwnPropertyDescriptor(Element.prototype, 'scrollTo');
  Object.defineProperty(Element.prototype, 'scrollTo', {
    configurable: true,
    writable: true,
    value(this: HTMLElement, options?: ScrollToOptions) {
      this.scrollTop = options?.top ?? this.scrollTop;
      setTimeout(() => {
        this.dispatchEvent(new Event('scroll'));
      }, 0);
    },
  });
  return () => {
    if (scrollTo === undefined) Reflect.deleteProperty(Element.prototype, 'scrollTo');
    else Object.defineProperty(Element.prototype, 'scrollTo', scrollTo);
    for (const { name, original } of originals) {
      if (original === undefined) Reflect.deleteProperty(HTMLElement.prototype, name);
      else Object.defineProperty(HTMLElement.prototype, name, original);
    }
  };
};

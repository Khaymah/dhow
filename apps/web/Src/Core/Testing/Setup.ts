import '@testing-library/jest-dom/vitest';

import { cleanup } from '@testing-library/react';
import { afterEach, beforeEach, vi } from 'vitest';

import { orgEventsRegistry } from '@/Core/Events/OrgEventsRegistry';
import { FakeEventSource } from '@/Core/Testing/FakeEventSource';
import { DEFAULT_THEME_ID } from '@/Core/Theme/ThemeTokens';
import { useCsrfStore } from '@/Core/Store/useCsrfStore';
import { useThemeStore } from '@/Core/Theme/useThemeStore';
import { loadBootMessages } from '@/I18n/BootCatalog';
import { loadWorkspaceMessages } from '@/I18n/WorkspaceCatalog';

await Promise.all([loadBootMessages('en'), loadWorkspaceMessages('ar'), loadWorkspaceMessages('en')]);

window.scrollTo = vi.fn();
Element.prototype.scrollIntoView = vi.fn();
Element.prototype.hasPointerCapture = vi.fn(() => false);
Element.prototype.setPointerCapture = vi.fn();
Element.prototype.releasePointerCapture = vi.fn();

class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal('EventSource', FakeEventSource);
  vi.stubGlobal('ResizeObserver', ResizeObserverStub);
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
});

afterEach(() => {
  cleanup();
  orgEventsRegistry.Reset();
  vi.unstubAllGlobals();
  window.localStorage.clear();
  useCsrfStore.getState().Clear();
  document.documentElement.lang = 'ar';
  document.documentElement.dir = 'rtl';
  document.documentElement.removeAttribute('data-theme');
  document.documentElement.removeAttribute('style');
  useThemeStore.setState({ ActiveId: DEFAULT_THEME_ID, CustomThemes: [], Draft: null });
});

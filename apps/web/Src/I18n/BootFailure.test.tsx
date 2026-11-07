import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { I18nProvider } from '@/I18n/I18nProvider';
import { STORAGE_KEY } from '@/I18n/Locale';
import { Ar } from '@/I18n/Locales/Ar';
import { useCatalogStore } from '@/I18n/useCatalogStore';
import { useI18n } from '@/I18n/useI18n';

vi.mock('@/I18n/BootCatalog', () => ({
  loadBootMessages: vi.fn(() => Promise.reject(new Error('chunk failed'))),
}));

const Probe = () => {
  const { Locale, SetLocale, t } = useI18n();
  return (
    <>
      <p>{t('home.title')}</p>
      <output>{Locale}</output>
      <button
        type="button"
        onClick={() => {
          SetLocale('en');
        }}
      >
        english
      </button>
    </>
  );
};

describe('a boot catalog that fails to load', () => {
  beforeEach(() => {
    useCatalogStore.setState({ Boot: {} });
    useToastStore.setState({ Toasts: [] });
    window.localStorage.clear();
  });

  it('starts fully in Arabic when English was saved but its catalog is missing', () => {
    window.localStorage.setItem(STORAGE_KEY, 'en');
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    expect(screen.getByRole('status')).toHaveTextContent('ar');
    expect(screen.getByText(Ar['home.title'])).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('ar');
    expect(document.documentElement.dir).toBe('rtl');
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('en');
  });

  it('keeps the current language and says so when a switch fails', async () => {
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    await userEvent.click(screen.getByRole('button', { name: 'english' }));
    await waitFor(() => {
      expect(useToastStore.getState().Toasts.map((toast) => toast.Title)).toEqual([Ar['i18n.switchFailed']]);
    });
    expect(screen.getByRole('status')).toHaveTextContent('ar');
    expect(document.documentElement.dir).toBe('rtl');
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });
});

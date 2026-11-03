import { screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { useToastStore } from '@/Core/Store/useToastStore';
import { renderWithProviders } from '@/Core/Testing/Render';
import { ACTIVE_THEME_KEY } from '@/Core/Theme/ThemeStorage';
import { DEFAULT_COLORS, SYSTEM_THEMES } from '@/Core/Theme/ThemeTokens';
import { useThemeStore } from '@/Core/Theme/useThemeStore';
import { Ar } from '@/I18n/Locales/Ar';
import { AppearanceBody } from '@/Modules/Appearance/Components/AppearanceBody';
import { isolate } from '@/I18n/Translate';

const html = document.documentElement;

const seed = (Id = 'mine', Name = 'Harbour') => {
  useThemeStore.setState({ CustomThemes: [{ Id, Name, Colors: { ...DEFAULT_COLORS, canvas: '#102030' } }] });
};

describe('AppearanceBody', () => {
  it('shows the four system themes with mini previews and marks the one in use', () => {
    renderWithProviders(<AppearanceBody onEdit={vi.fn()} />);
    for (const theme of SYSTEM_THEMES) {
      const card = screen.getByRole('button', { name: Ar[`theme.system.${theme.Id}`] });
      expect(card).toHaveAttribute('aria-pressed', theme.Id === 'sea' ? 'true' : 'false');
      expect(card.querySelector(`[data-theme="${theme.Id}"]`)).not.toBeNull();
    }
  });

  it('switches the theme, remembers it and says so', async () => {
    const { user } = renderWithProviders(<AppearanceBody onEdit={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: Ar['theme.system.teak'] }));
    expect(html).toHaveAttribute('data-theme', 'teak');
    expect(window.localStorage.getItem(ACTIVE_THEME_KEY)).toBe('teak');
    expect(screen.getByRole('button', { name: Ar['theme.system.teak'] })).toHaveAttribute('aria-pressed', 'true');
    expect(useToastStore.getState().Toasts.map((toast) => toast.Title)).toContain(
      Ar['appearance.changed'].replace('{name}', isolate(Ar['theme.system.teak'])),
    );
  });

  it('says there are no custom themes yet', () => {
    renderWithProviders(<AppearanceBody onEdit={vi.fn()} />);
    expect(screen.getByText(Ar['appearance.mine.empty'])).toBeInTheDocument();
  });

  it('lists custom themes with Use and Edit', async () => {
    seed();
    const onEdit = vi.fn();
    const { user } = renderWithProviders(<AppearanceBody onEdit={onEdit} />);
    const row = screen.getByText('Harbour').closest('li') as HTMLElement;
    expect(within(row).getByText(Ar['appearance.custom'])).toBeInTheDocument();
    await user.click(within(row).getByRole('button', { name: Ar['appearance.use'] }));
    expect(useThemeStore.getState().ActiveId).toBe('mine');
    expect(html.style.getPropertyValue('--color-canvas')).toBe('#102030');
    expect(html).not.toHaveAttribute('data-theme');
    expect(within(row).getByText(Ar['appearance.inUse'])).toBeInTheDocument();
    await user.click(within(row).getByRole('button', { name: Ar['appearance.edit'] }));
    expect(onEdit).toHaveBeenCalledWith('mine');
  });
});

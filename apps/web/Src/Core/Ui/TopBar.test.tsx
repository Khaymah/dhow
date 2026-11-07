import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderUi } from '@/Core/Testing/Render';
import { TopBar } from '@/Core/Ui/TopBar';

describe('TopBar', () => {
  it('opens search from the button and shows the shortcut hint', async () => {
    const onSearch = vi.fn();
    const { user } = renderUi(<TopBar searchLabel="Search" shortcut="Ctrl K" onSearch={onSearch} />);
    await user.click(screen.getByRole('button', { name: 'Search' }));
    expect(onSearch).toHaveBeenCalledTimes(1);
    expect(screen.getByText('Ctrl K')).toBeInTheDocument();
  });

  it('names the search button by its visible label and keeps the shortcut out of the name', () => {
    renderUi(<TopBar searchLabel="Search" shortcut="Ctrl K" onSearch={vi.fn()} />);
    const button = screen.getByRole('button', { name: 'Search' });
    expect(button).not.toHaveAttribute('aria-label');
    expect(screen.getByText('Ctrl K')).toHaveAttribute('aria-hidden', 'true');
  });

  it('places the start slot before search and the controls at the end', () => {
    renderUi(
      <TopBar start={<span>Brand</span>} searchLabel="Search" shortcut="Ctrl K" onSearch={vi.fn()}>
        <button type="button">Bell</button>
      </TopBar>,
    );
    const order = screen.getAllByText(/Brand|Search|Bell/).map((node) => node.textContent);
    expect(order[0]).toBe('Brand');
    expect(order.at(-1)).toBe('Bell');
  });
});

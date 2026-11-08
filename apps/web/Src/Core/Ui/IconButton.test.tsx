import { screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderUi } from '@/Core/Testing/Render';
import { IconButton } from '@/Core/Ui/IconButton';

describe('IconButton', () => {
  it('is named by its label and fires onClick', async () => {
    const onClick = vi.fn();
    const { user } = renderUi(<IconButton icon="bell" label="Open notifications" onClick={onClick} />);
    await user.click(screen.getByRole('button', { name: 'Open notifications' }));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('shows the unread dot without changing the accessible name', () => {
    const { container } = renderUi(<IconButton icon="bell" label="Open notifications" badge />);
    expect(screen.getByRole('button', { name: 'Open notifications' })).toBeInTheDocument();
    expect(container.querySelector('i')).toHaveAttribute('aria-hidden', 'true');
  });

  it('puts the visible unread count in the accessible name, and caps it at 99', () => {
    const { rerender } = renderUi(<IconButton icon="bell" label="Open notifications, unread:" count={3} />);
    const button = screen.getByRole('button', { name: 'Open notifications, unread: 3' });
    expect(button).not.toHaveAttribute('aria-label');
    rerender(<IconButton icon="bell" label="Open notifications, unread:" count={250} />);
    expect(screen.getByRole('button', { name: 'Open notifications, unread: 99+' })).toBeInTheDocument();
    rerender(<IconButton icon="bell" label="Open notifications" count={0} />);
    expect(screen.getByRole('button', { name: 'Open notifications' })).toHaveAttribute('aria-label', 'Open notifications');
  });

  it('has a thin line, and drops it when plain', () => {
    renderUi(
      <>
        <IconButton icon="close" label="Close" />
        <IconButton icon="close" label="Dismiss" plain />
      </>,
    );
    expect(screen.getByRole('button', { name: 'Close' })).toHaveClass('border-line-2', 'rounded-btn');
    expect(screen.getByRole('button', { name: 'Dismiss' })).toHaveClass('border-transparent');
  });
});

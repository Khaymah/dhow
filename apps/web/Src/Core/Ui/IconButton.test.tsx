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

  it('shows the unread count hidden from readers, and caps it at 99', () => {
    const { container, rerender } = renderUi(<IconButton icon="bell" label="Open notifications, 3 unread" count={3} />);
    expect(screen.getByRole('button', { name: 'Open notifications, 3 unread' })).toBeInTheDocument();
    expect(container.querySelector('span[aria-hidden="true"]')).toHaveTextContent('3');
    rerender(<IconButton icon="bell" label="Open notifications" count={250} />);
    expect(container.querySelector('span[aria-hidden="true"]')).toHaveTextContent('99+');
    rerender(<IconButton icon="bell" label="Open notifications" count={0} />);
    expect(container.querySelector('span[aria-hidden="true"]')).toBeNull();
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

import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderUi } from '@/Core/Testing/Render';
import { PageHeader } from '@/Core/Ui/PageHeader';

describe('PageHeader', () => {
  it('shows the page title as the only level-one heading, with its line of context and one action', () => {
    renderUi(<PageHeader title="Jobs" lead="Every job a runner took." action={<button type="button">Add</button>} />);
    const heading = screen.getByRole('heading', { level: 1, name: 'Jobs' });
    expect(heading).toHaveClass('text-title-page');
    expect(screen.getByText('Every job a runner took.')).toBeInTheDocument();
    expect(screen.getAllByRole('button')).toHaveLength(1);
  });

  it('stays a bare title when there is no context and no action', () => {
    const { container } = renderUi(<PageHeader title="Home" />);
    expect(screen.getByRole('heading', { level: 1, name: 'Home' })).toBeInTheDocument();
    expect(container.querySelector('p')).toBeNull();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('keeps an empty help slot after the actions that hides itself until help arrives', () => {
    const { container } = renderUi(<PageHeader title="Home" action={<button type="button">Add</button>} />);
    const slot = container.querySelector('[data-slot="help"]');
    expect(slot).not.toBeNull();
    expect(slot).toBeEmptyDOMElement();
    expect(slot).toHaveClass('empty:hidden');
    expect(slot?.previousElementSibling).toContainElement(screen.getByRole('button', { name: 'Add' }));
  });

  it('puts the help button in the same slot, last in the tab order', async () => {
    const { container, user } = renderUi(
      <PageHeader title="Home" action={<button type="button">Add</button>} help={<button type="button">Help</button>} />,
    );
    const slot = container.querySelector('[data-slot="help"]');
    expect(slot).toContainElement(screen.getByRole('button', { name: 'Help' }));
    await user.tab();
    expect(screen.getByRole('button', { name: 'Add' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Help' })).toHaveFocus();
  });
});

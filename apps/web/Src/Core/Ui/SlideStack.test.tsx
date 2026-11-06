import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { findBackdrop } from '@/Core/Testing/Overlay';
import { renderWithProviders } from '@/Core/Testing/Render';
import { SlideStack, type TJumpSource } from '@/Core/Ui/SlideStack';
import { Ar } from '@/I18n/Locales/Ar';
import { isolate } from '@/I18n/Translate';

const TITLES: Record<string, string> = { a: 'Alpha', b: 'Bravo', c: 'Charlie', d: 'Delta', e: 'Echo', f: 'Foxtrot' };
const NEXT: Record<string, string | undefined> = { a: 'b', b: 'c', c: 'd', d: 'e', e: 'f' };

interface IHarnessProps {
  initial?: string[];
  onJump?: (index: number, source: TJumpSource) => void;
}

const Harness = ({ initial = [], onJump }: IHarnessProps) => {
  const [Ids, setIds] = useState<string[]>(initial);
  const slides = Ids.map((id) => {
    const next = NEXT[id];
    return {
      id,
      title: TITLES[id] ?? id,
      subtitle: <span>About {id}</span>,
      footer: <button type="button">Footer {id}</button>,
      children: (
        <div>
          <p>Body {id}</p>
          {next ? (
            <button
              type="button"
              onClick={() => {
                setIds((current) => [...current, next]);
              }}
            >
              Open {next}
            </button>
          ) : null}
        </div>
      ),
    };
  });

  return (
    <>
      <button
        type="button"
        onClick={() => {
          setIds(['a']);
        }}
      >
        Opener
      </button>
      <SlideStack
        slides={slides}
        onPop={() => {
          setIds((current) => current.slice(0, -1));
        }}
        onJump={(index, source) => {
          onJump?.(index, source);
          setIds((current) => current.slice(0, index + 1));
        }}
        onCloseAll={() => {
          setIds([]);
        }}
      />
    </>
  );
};

const topSlide = () => {
  const dialog = screen.getByRole('dialog');
  const sections = [...dialog.querySelectorAll('section')].filter((section) => !section.hasAttribute('inert'));
  const section = sections[sections.length - 1];
  if (!section) throw new Error('there is no top slide');
  return section;
};

const livePanels = () => [...document.querySelectorAll('[data-slide-level]:not([data-leaving])')];
const gone = async () => {
  await waitFor(() => {
    expect(document.querySelector('[data-slide-level]')).toBeNull();
  });
};

describe('SlideStack', () => {
  it('shows nothing for an empty stack and a labelled dialog for a stack', async () => {
    const { user } = renderWithProviders(<Harness />);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Opener' }));
    expect(await screen.findByRole('dialog', { name: 'Alpha' })).toBeInTheDocument();
    expect(screen.getByText('Body a')).toBeInTheDocument();
    expect(screen.getByText('About a')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Footer a' })).toBeInTheDocument();
  });

  it('pushes a slide on top and pops back with the back button', async () => {
    const { user } = renderWithProviders(<Harness initial={['a']} />);
    expect(screen.queryByRole('button', { name: Ar['slides.back'] })).not.toBeInTheDocument();
    await user.click(await screen.findByRole('button', { name: 'Open b' }));
    expect(await screen.findByRole('dialog', { name: 'Bravo' })).toBeInTheDocument();
    expect(livePanels()).toHaveLength(2);
    await user.click(within(topSlide()).getByRole('button', { name: Ar['slides.back'] }));
    expect(await screen.findByRole('dialog', { name: 'Alpha' })).toBeInTheDocument();
    expect(livePanels()).toHaveLength(1);
  });

  it('pops exactly one level on Escape', async () => {
    const { user } = renderWithProviders(<Harness initial={['a', 'b', 'c']} />);
    await screen.findByRole('dialog', { name: 'Charlie' });
    await user.keyboard('{Escape}');
    expect(await screen.findByRole('dialog', { name: 'Bravo' })).toBeInTheDocument();
    expect(livePanels()).toHaveLength(2);
    await user.keyboard('{Escape}');
    expect(await screen.findByRole('dialog', { name: 'Alpha' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await gone();
  });

  it('pops exactly one level when the backdrop is pressed', async () => {
    renderWithProviders(<Harness initial={['a', 'b']} />);
    await screen.findByRole('dialog', { name: 'Bravo' });
    fireEvent.click(findBackdrop());
    expect(await screen.findByRole('dialog', { name: 'Alpha' })).toBeInTheDocument();
    expect(livePanels()).toHaveLength(1);
  });

  it('returns to a lower slide when it is pressed and drops everything above it', async () => {
    const onJump = vi.fn();
    renderWithProviders(<Harness initial={['a', 'b', 'c']} onJump={onJump} />);
    await screen.findByRole('dialog', { name: 'Charlie' });
    const targets = document.querySelectorAll<HTMLElement>('[data-slide-return]');
    expect(targets).toHaveLength(2);
    fireEvent.click(targets.item(0));
    expect(onJump).toHaveBeenCalledWith(0, 'slide');
    expect(await screen.findByRole('dialog', { name: 'Alpha' })).toBeInTheDocument();
    expect(livePanels()).toHaveLength(1);
  });

  it('lists every level in the trail and jumps to a level from its crumb', async () => {
    const onJump = vi.fn();
    const { user } = renderWithProviders(<Harness initial={['a', 'b', 'c']} onJump={onJump} />);
    await screen.findByRole('dialog', { name: 'Charlie' });
    const trail = within(topSlide()).getByRole('navigation', { name: Ar['slides.trail'] });
    const crumbs = within(trail).getAllByRole('listitem');
    expect(crumbs.map((crumb) => crumb.textContent)).toEqual(['Alpha', 'Bravo', 'Charlie']);
    expect(within(trail).getAllByRole('button')).toHaveLength(2);
    expect(within(trail).getByText('Charlie')).toHaveAttribute('aria-current', 'step');
    await user.click(within(trail).getByRole('button', { name: 'Alpha' }));
    expect(onJump).toHaveBeenCalledWith(0, 'crumb');
    expect(await screen.findByRole('dialog', { name: 'Alpha' })).toBeInTheDocument();
  });

  it('shows each slide its own place in the stack', async () => {
    renderWithProviders(<Harness initial={['a', 'b', 'c']} />);
    await screen.findByRole('dialog', { name: 'Charlie' });
    for (const label of [1, 2, 3].map((n) => `${isolate(n)} من ${isolate('3')}`)) {
      expect(document.body).toHaveTextContent(label);
    }
    expect(within(topSlide()).getByText(`${isolate('3')} من ${isolate('3')}`)).toBeInTheDocument();
  });

  it('closes every slide at once', async () => {
    const { user } = renderWithProviders(<Harness initial={['a', 'b', 'c']} />);
    await screen.findByRole('dialog', { name: 'Charlie' });
    await user.click(within(topSlide()).getByRole('button', { name: Ar['slides.closeAll'] }));
    await gone();
  });
});

describe('SlideStack in direction A', () => {
  it('leaves out the trail while only one slide is open, since it would repeat the title', async () => {
    renderWithProviders(<Harness initial={['a']} />);
    await screen.findByRole('dialog', { name: 'Alpha' });
    expect(within(topSlide()).queryByRole('navigation', { name: Ar['slides.trail'] })).not.toBeInTheDocument();
  });

  it('starts beside the side menu, with the offset and the width taken from the layout tokens', async () => {
    renderWithProviders(<Harness initial={['a']} />);
    const dialog = await screen.findByRole('dialog', { name: 'Alpha' });
    expect(dialog).toHaveClass('start-menu', 'max-md:bottom-bottombar');
    expect(dialog.className).toContain('var(--spacing-menu)');
    expect(dialog.className).toContain('var(--spacing-panel)');
    expect(dialog.className).not.toContain('rail');
  });

  it('draws the slide flat on the canvas with a thin line and 16px corners', async () => {
    renderWithProviders(<Harness initial={['a']} />);
    await screen.findByRole('dialog', { name: 'Alpha' });
    expect(topSlide()).toHaveClass('rounded-lg', 'border-line-2', 'bg-canvas');
  });
});

describe('SlideStack accessibility', () => {
  it('makes the lower slides inert and hidden from assistive technology', async () => {
    renderWithProviders(<Harness initial={['a', 'b', 'c']} />);
    await screen.findByRole('dialog', { name: 'Charlie' });
    const sections = [...document.querySelectorAll('[data-slide-level] > section')];
    expect(sections).toHaveLength(3);
    const [first, second, third] = sections;
    for (const lower of [first, second]) {
      expect(lower).toHaveAttribute('inert');
      expect(lower).toHaveAttribute('aria-hidden', 'true');
    }
    expect(third).not.toHaveAttribute('inert');
    expect(third).not.toHaveAttribute('aria-hidden');
    expect(screen.queryByRole('button', { name: 'Footer a' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Footer c' })).toBeInTheDocument();
  });

  it('shifts lower slides by depth and hides everything deeper than four levels', async () => {
    renderWithProviders(<Harness initial={['a', 'b', 'c', 'd', 'e', 'f']} />);
    await screen.findByRole('dialog', { name: 'Foxtrot' });
    const panels = [...document.querySelectorAll<HTMLElement>('[data-slide-level]')];
    expect(panels.map((panel) => panel.style.getPropertyValue('--depth'))).toEqual(['5', '4', '3', '2', '1', '0']);
    expect(panels.map((panel) => panel.hasAttribute('data-gone'))).toEqual([true, true, false, false, false, false]);
  });

  it('moves focus into the top slide, into each new top slide and back to the opener row on pop', async () => {
    const { user } = renderWithProviders(<Harness />);
    await user.click(screen.getByRole('button', { name: 'Opener' }));
    await waitFor(() => {
      expect(topSlide()).toContainElement(document.activeElement as HTMLElement);
    });
    const open = screen.getByRole('button', { name: 'Open b' });
    await user.click(open);
    await screen.findByRole('dialog', { name: 'Bravo' });
    await waitFor(() => {
      expect(topSlide()).toContainElement(document.activeElement as HTMLElement);
      expect(within(topSlide()).getByText('Body b')).toBeInTheDocument();
    });
    await user.keyboard('{Escape}');
    await waitFor(() => {
      expect(open).toHaveFocus();
    });
  });

  it('returns focus to the control that opened the stack once it is empty', async () => {
    const { user } = renderWithProviders(<Harness />);
    const opener = screen.getByRole('button', { name: 'Opener' });
    await user.click(opener);
    await user.click(await screen.findByRole('button', { name: 'Open b' }));
    await screen.findByRole('dialog', { name: 'Bravo' });
    await user.click(within(topSlide()).getByRole('button', { name: Ar['slides.closeAll'] }));
    await gone();
    await waitFor(() => {
      expect(opener).toHaveFocus();
    });
  });

  it('keeps Tab and Shift+Tab inside the top slide', async () => {
    const { user } = renderWithProviders(<Harness initial={['a', 'b']} />);
    await screen.findByRole('dialog', { name: 'Bravo' });
    const section = topSlide();
    section.focus();
    for (let press = 0; press < 9; press += 1) {
      await user.tab();
      expect(section).toContainElement(document.activeElement as HTMLElement);
    }
    for (let press = 0; press < 9; press += 1) {
      await user.tab({ shift: true });
      expect(section).toContainElement(document.activeElement as HTMLElement);
    }
  });
});

describe('SlideStack motion', () => {
  it('keeps nothing of the body of a leaving slide, so a value shown once goes the moment it closes', async () => {
    const { user } = renderWithProviders(<Harness initial={['a', 'b']} />);
    await screen.findByRole('dialog', { name: 'Bravo' });
    expect(screen.getByText('Body b')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    const leaving = document.querySelector('[data-leaving]');
    expect(leaving).not.toBeNull();
    expect(leaving?.textContent ?? '').not.toContain('Body b');
    expect(screen.queryByText('Body b')).toBeNull();
  });

  it('animates a leaving slide out and removes it afterwards', async () => {
    const { user } = renderWithProviders(<Harness initial={['a', 'b']} />);
    await screen.findByRole('dialog', { name: 'Bravo' });
    await user.keyboard('{Escape}');
    expect(document.querySelector('[data-leaving]')).not.toBeNull();
    await waitFor(
      () => {
        expect(document.querySelector('[data-leaving]')).toBeNull();
      },
      { timeout: 1500 },
    );
    expect(livePanels()).toHaveLength(1);
  });

  it('drops the exit delay and starts in place when the person prefers reduced motion', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        matches: true,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    const { user } = renderWithProviders(<Harness initial={['a', 'b']} />);
    await screen.findByRole('dialog', { name: 'Bravo' });
    for (const panel of document.querySelectorAll('[data-slide-level]'))
      expect(panel).toHaveAttribute('data-in', 'true');
    await user.keyboard('{Escape}');
    await act(async () => {
      await Promise.resolve();
    });
    await waitFor(
      () => {
        expect(document.querySelector('[data-leaving]')).toBeNull();
      },
      { timeout: 100 },
    );
  });

  it('never injects a style element, so a strict style-src policy holds', async () => {
    const before = document.querySelectorAll('style').length;
    const { user } = renderWithProviders(<Harness initial={['a']} />);
    await user.click(await screen.findByRole('button', { name: 'Open b' }));
    await screen.findByRole('dialog', { name: 'Bravo' });
    expect(document.querySelectorAll('style')).toHaveLength(before);
  });
});

describe('SlideStack close guard', () => {
  const Guarded = ({ blocked, onBlocked, onClose }: { blocked: boolean; onBlocked: () => void; onClose: () => void }) => (
    <SlideStack
      slides={[{ id: 'a', title: 'Alpha', children: <p>Body a</p> }]}
      onPop={onClose}
      onJump={onClose}
      onCloseAll={onClose}
      closeGuard={{ blocked, onBlocked }}
    />
  );

  it('asks the owner of the slide instead of closing on Escape, the backdrop or the close button', async () => {
    const onBlocked = vi.fn();
    const onClose = vi.fn();
    const view = renderWithProviders(<Guarded blocked onBlocked={onBlocked} onClose={onClose} />);
    const dialog = await screen.findByRole('dialog', { name: 'Alpha' });
    fireEvent.keyDown(dialog, { key: 'Escape' });
    fireEvent.click(findBackdrop());
    await view.user.click(within(dialog).getByRole('button', { name: Ar['slides.closeAll'] }));
    expect(onBlocked).toHaveBeenCalledTimes(3);
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector('style')).toBeNull();
  });

  it('closes as usual while nothing is guarded', async () => {
    const onBlocked = vi.fn();
    const onClose = vi.fn();
    const view = renderWithProviders(<Guarded blocked={false} onBlocked={onBlocked} onClose={onClose} />);
    const dialog = await screen.findByRole('dialog', { name: 'Alpha' });
    await view.user.click(within(dialog).getByRole('button', { name: Ar['slides.closeAll'] }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onBlocked).not.toHaveBeenCalled();
  });
});

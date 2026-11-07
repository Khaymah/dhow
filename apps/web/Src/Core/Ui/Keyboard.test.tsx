import { screen } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { renderUi } from '@/Core/Testing/Render';
import { Button } from '@/Core/Ui/Button';
import { CheckList } from '@/Core/Ui/CheckList';
import { ChoiceList } from '@/Core/Ui/ChoiceList';
import { IconButton } from '@/Core/Ui/IconButton';
import { Tabs } from '@/Core/Ui/Tabs';

type TTab = 'one' | 'two' | 'three';

const TABS = [
  { id: 'one', label: 'One' },
  { id: 'two', label: 'Two' },
  { id: 'three', label: 'Three' },
] as const;

const Flow = ({ onSubmit }: { onSubmit: () => void }) => {
  const [Tab, setTab] = useState<TTab>('one');
  const [Choice, setChoice] = useState<string | null>('a');
  const [Boxes, setBoxes] = useState<readonly string[]>([]);
  return (
    <main>
      <IconButton icon="bell" label="Notifications" />
      <Tabs base="flow" label="Sections" tabs={TABS} value={Tab} onChange={setTab} />
      <ChoiceList
        label="Mode"
        value={Choice}
        onChange={setChoice}
        choices={[
          { value: 'a', title: 'Alpha' },
          { value: 'b', title: 'Beta' },
        ]}
      />
      <CheckList
        label="Targets"
        values={Boxes}
        onChange={(values) => {
          setBoxes(values);
        }}
        items={[{ value: 'x', title: 'Ex' }]}
      />
      <Button variant="primary" onClick={onSubmit}>
        Save
      </Button>
    </main>
  );
};

describe('keyboard only', () => {
  it('reaches every control in order and activates them with Enter and Space', async () => {
    const onSubmit = vi.fn();
    const { user } = renderUi(<Flow onSubmit={onSubmit} />);
    await user.tab();
    expect(screen.getByRole('button', { name: 'Notifications' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('tab', { name: 'One' })).toHaveFocus();
    await user.keyboard('{End}');
    expect(screen.getByRole('tab', { name: 'Three' })).toHaveFocus();
    expect(screen.getByRole('tab', { name: 'Three' })).toHaveAttribute('aria-selected', 'true');
    await user.tab();
    expect(screen.getByRole('radio', { name: /Alpha/ })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('radio', { name: /Beta/ })).toHaveFocus();
    await user.keyboard(' ');
    expect(screen.getByRole('radio', { name: /Beta/ })).toHaveAttribute('aria-checked', 'true');
    await user.tab();
    expect(screen.getByRole('checkbox', { name: /Ex/ })).toHaveFocus();
    await user.keyboard(' ');
    expect(screen.getByRole('checkbox', { name: /Ex/ })).toHaveAttribute('aria-checked', 'true');
    await user.tab();
    expect(screen.getByRole('button', { name: 'Save' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it('goes back with Shift+Tab', async () => {
    const { user } = renderUi(<Flow onSubmit={vi.fn()} />);
    await user.tab({ shift: true });
    expect(screen.getByRole('button', { name: 'Save' })).toHaveFocus();
  });
});

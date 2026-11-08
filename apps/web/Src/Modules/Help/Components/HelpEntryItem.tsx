import { useId, useState } from 'react';

import type { IHelpEntry } from '@/Core/Help/HelpTypes';
import { cx } from '@/Core/Ui/ClassNames';
import { Icon } from '@/Core/Ui/Icon';
import { HelpBlocks } from '@/Modules/Help/Components/HelpBlocks';

export interface IHelpEntryItemProps {
  entry: IHelpEntry;
  expanded?: boolean;
  context?: string | null;
}

export const HelpEntryItem = ({ entry, expanded = false, context = null }: IHelpEntryItemProps) => {
  const [Open, setOpen] = useState(false);
  const bodyId = useId();
  const open = expanded || Open;

  return (
    <article className="border-t border-line first:border-t-0">
      <h4 className="m-0">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={() => {
            setOpen(!Open);
          }}
          className="flex min-h-11 w-full cursor-pointer items-center gap-3 border-0 bg-transparent px-card py-3 text-start text-label text-primary hover:bg-hover"
        >
          <span className="grid min-w-0 flex-1 gap-0.5">
            {context ? <span className="text-body-sm text-tertiary">{context}</span> : null}
            <bdi>{entry.Title}</bdi>
          </span>
          <Icon name="plus" size={16} className={cx('flex-none text-secondary transition-transform duration-140', open && 'rotate-45')} />
        </button>
      </h4>
      <div id={bodyId} hidden={!open} className="px-card pb-card">
        <HelpBlocks blocks={entry.Body} />
      </div>
    </article>
  );
};

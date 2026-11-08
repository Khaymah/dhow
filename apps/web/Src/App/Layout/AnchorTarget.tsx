import type { ReactNode } from 'react';

import { useAnchorTarget } from '@/App/Layout/useAnchorTarget';

export interface IAnchorTargetProps {
  anchor: string;
  children: ReactNode;
}

export const AnchorTarget = ({ anchor, children }: IAnchorTargetProps) => {
  const ref = useAnchorTarget(anchor);
  return (
    <div ref={ref} id={anchor} tabIndex={-1} className="min-w-0 scroll-mt-4">
      {children}
    </div>
  );
};

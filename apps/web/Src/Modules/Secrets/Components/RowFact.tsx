import type { ReactNode } from 'react';

import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';

export const RowFact = ({ label, children, wide = false }: { label: string; children: ReactNode; wide?: boolean }) => (
  <div className={wide ? 'col-span-2 grid gap-1 sm:col-span-3' : 'grid gap-0.5'}>
    <dt className="text-body-sm text-tertiary">{label}</dt>
    <dd className="m-0 min-w-0 text-body-sm text-primary">{children}</dd>
  </div>
);

export const AgoTime = ({ iso }: { iso: string }) => {
  const time = useRelativeTime();
  return (
    <time dateTime={iso} title={time.full(iso)}>
      {time.ago(iso)}
    </time>
  );
};

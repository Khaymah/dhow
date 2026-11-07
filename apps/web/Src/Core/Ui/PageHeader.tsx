import type { ReactNode } from 'react';

export interface IPageHeaderProps {
  title: string;
  lead?: ReactNode;
  action?: ReactNode;
  help?: ReactNode;
}

export const PageHeader = ({ title, lead, action, help }: IPageHeaderProps) => (
  <header className="flex flex-wrap items-start gap-x-4 gap-y-3">
    <div className="grid min-w-0 flex-1 gap-1">
      <h1 className="m-0 text-title-page">
        <bdi>{title}</bdi>
      </h1>
      {lead ? <p className="m-0 flex flex-wrap items-center gap-x-3 gap-y-1 text-body-sm text-secondary">{lead}</p> : null}
    </div>
    {action ? <div className="flex flex-none flex-wrap items-center gap-2">{action}</div> : null}
    <div data-slot="help" className="flex flex-none items-center empty:hidden">
      {help}
    </div>
  </header>
);

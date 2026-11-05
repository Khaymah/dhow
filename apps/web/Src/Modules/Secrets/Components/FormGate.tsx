import type { ReactNode } from 'react';

import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useT } from '@/I18n/useT';
import type { ISecretScope } from '@/Modules/Secrets/Api/SecretsApi';
import { useSecretScope } from '@/Modules/Secrets/Hooks/useSecretScope';

export interface IFormGateProps {
  projectId: string | undefined;
  children: (scope: ISecretScope) => ReactNode;
}

export const FormGate = ({ projectId, children }: IFormGateProps) => {
  const t = useT();
  const access = useSecretScope(projectId);
  if (access.Kind === 'loading') return <Skeleton label={t('ui.loading')} lines={5} />;
  if (access.Kind === 'denied') return <InlineAlert accent="awaiting" title={t('secrets.form.forbidden')} />;
  return <>{children(access.Scope)}</>;
};

export interface IFormLookupProps<TItem> {
  query: { data: TItem[] | undefined; isError: boolean; error: unknown };
  find: (items: TItem[]) => TItem | undefined;
  missing: string;
  children: (item: TItem) => ReactNode;
}

export const FormLookup = <TItem,>({ query, find, missing, children }: IFormLookupProps<TItem>) => {
  const t = useT();
  const errorText = useApiErrorText();
  if (query.isError) return <InlineAlert accent="failed" title={errorText(query.error)} />;
  if (query.data === undefined) return <Skeleton label={t('ui.loading')} lines={5} />;
  const item = find(query.data);
  if (item === undefined) return <InlineAlert accent="awaiting" title={missing} />;
  return <>{children(item)}</>;
};

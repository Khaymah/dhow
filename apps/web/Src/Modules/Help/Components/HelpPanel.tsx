import { useDeferredValue, useMemo, useState } from 'react';

import { docsUrl } from '@/Core/Help/DocsUrl';
import { buildIndex, queryTerms, searchEntries } from '@/Core/Help/HelpSearch';
import type { IHelpContent } from '@/Core/Help/HelpTypes';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { Field } from '@/Core/Ui/Field';
import { IconButton } from '@/Core/Ui/IconButton';
import { Input } from '@/Core/Ui/Input';
import { useI18n } from '@/I18n/useI18n';
import { HelpEntryItem } from '@/Modules/Help/Components/HelpEntryItem';
import { ReleasesCard } from '@/Modules/Help/Components/ReleasesCard';

export interface IHelpPanelProps {
  content: IHelpContent;
}

const SearchResults = ({ content, query }: { content: IHelpContent; query: string }) => {
  const { Locale, t } = useI18n();
  const index = useMemo(() => buildIndex(content), [content]);
  const hits = useMemo(() => searchEntries(index, query), [index, query]);
  const fallback = docsUrl('faq.md', Locale);

  return (
    <>
      <p role="status" className="m-0 text-body-sm text-secondary">
        {t('help.search.results', { count: hits.length })}
      </p>
      {hits.length === 0 ? (
        <Card>
          <EmptyState
            icon="help"
            text={t('help.search.empty', { query: query.trim() })}
            action={
              fallback ? (
                <a href={fallback} target="_blank" rel="noreferrer noopener" className="text-primary underline underline-offset-4">
                  {t('help.docs.all')}
                  <span className="sr-only">{` (${t('api.error.newTab')})`}</span>
                </a>
              ) : null
            }
          />
        </Card>
      ) : (
        <Card>
          {hits.map((hit) => (
            <HelpEntryItem key={hit.Entry.Id} entry={hit.Entry} expanded context={hit.Group ?? t('help.troubleshooting.title')} />
          ))}
        </Card>
      )}
    </>
  );
};

const Browse = ({ content }: { content: IHelpContent }) => {
  const t = useI18n().t;
  return (
    <>
      {content.Faq.map((group) => (
        <Card key={group.Id} title={group.Title} icon="help">
          {group.Entries.map((entry) => (
            <HelpEntryItem key={entry.Id} entry={entry} />
          ))}
        </Card>
      ))}
      <Card title={t('help.troubleshooting.title')} icon="alert">
        {content.Troubleshooting.map((entry) => (
          <HelpEntryItem key={entry.Id} entry={entry} />
        ))}
      </Card>
      <ReleasesCard releases={content.Releases} />
    </>
  );
};

export const HelpPanel = ({ content }: IHelpPanelProps) => {
  const { t } = useI18n();
  const [Query, setQuery] = useState('');
  const deferred = useDeferredValue(Query);
  const searching = queryTerms(deferred).length > 0;

  return (
    <div className="grid gap-block">
      <div className="flex items-end gap-2">
        <Field label={t('help.search.label')} className="min-w-0 max-w-xl flex-1">
          <Input
            type="search"
            value={Query}
            placeholder={t('help.search.placeholder')}
            autoComplete="off"
            onChange={(event) => {
              setQuery(event.target.value);
            }}
          />
        </Field>
        {Query === '' ? null : (
          <IconButton
            icon="close"
            label={t('help.search.clear')}
            onClick={() => {
              setQuery('');
            }}
          />
        )}
      </div>
      {searching ? <SearchResults content={content} query={deferred} /> : <Browse content={content} />}
    </div>
  );
};

import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Pager } from '@/Core/Ui/Pager';
import { Rows } from '@/Core/Ui/Row';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { RUNS_PAGE_SIZE } from '@/Modules/Projects/Api/RunsApi';
import { FilterChips } from '@/Modules/Projects/Components/FilterChips';
import { RunRow } from '@/Modules/Projects/Components/RunRow';
import { useNow } from '@/Modules/Projects/Hooks/useNow';
import { useRuns } from '@/Modules/Projects/Hooks/useRuns';
import { isFinalRun, RUN_FILTERS, statesOf, type TRunFilter } from '@/Modules/Projects/Types/RunStates';

export interface IRunsCardProps {
  organizationId: string;
  projectId: string;
  hrefFor: (runId: string) => string;
  onOpenRun: (runId: string) => void;
}

export const RunsCard = ({ organizationId, projectId, hrefFor, onOpenRun }: IRunsCardProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const [Filter, setFilter] = useState<TRunFilter>('all');
  const [Page, setPage] = useState(1);
  const runs = useRuns(organizationId, projectId, Page, statesOf(Filter));
  const active = runs.data?.Items.some((run) => !isFinalRun(run.State)) ?? false;
  const now = useNow(active);

  const filters = (
    <FilterChips
      label={t('runs.filter.label')}
      chips={RUN_FILTERS.map((filter) => ({ id: filter, label: t(`runs.filter.${filter}`) }))}
      value={Filter}
      disabled={runs.isFetching && runs.data === undefined}
      onChange={(next) => {
        setFilter(next);
        setPage(1);
      }}
    />
  );

  let body;
  if (runs.isError) {
    body = (
      <div className="grid justify-items-start gap-3 px-card pb-card">
        <InlineAlert accent="failed" title={t('runs.error')}>
          {errorText(runs.error)}
        </InlineAlert>
        <Button size="sm" onClick={() => void runs.refetch()}>
          {t('projects.retry')}
        </Button>
      </div>
    );
  } else if (runs.data === undefined) {
    body = (
      <div className="px-card pb-card">
        <Skeleton label={t('runs.loading')} lines={4} />
      </div>
    );
  } else if (runs.data.Items.length === 0) {
    body = <EmptyState icon="play" text={Filter === 'all' ? t('runs.empty') : t('runs.empty.filtered')} className="pb-card" />;
  } else {
    body = (
      <Rows label={t('runs.title')}>
        {runs.data.Items.map((run) => (
          <RunRow key={run.Id} run={run} now={now} href={hrefFor(run.Id)} onOpen={() => { onOpenRun(run.Id); }} />
        ))}
      </Rows>
    );
  }

  return (
    <div className="grid gap-3">
      {filters}
      <Card title={t('runs.title')} icon="play" end={runs.data === undefined ? undefined : t('runs.total', { count: runs.data.Meta.Total })}>
        {body}
      </Card>
      {runs.data === undefined ? null : (
        <Pager page={runs.data.Meta.Page} limit={runs.data.Meta.Limit || RUNS_PAGE_SIZE} total={runs.data.Meta.Total} busy={runs.isFetching} onPage={setPage} />
      )}
    </div>
  );
};

import { useState } from 'react';

import type { TRole } from '@/Core/Auth/AuthTypes';
import { Card } from '@/Core/Ui/Card';
import { cx } from '@/Core/Ui/ClassNames';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { useT } from '@/I18n/useT';
import type { TJobDetail } from '@/Modules/Jobs/Api/JobsApi';
import { JobLog } from '@/Modules/Jobs/Components/JobLog';
import { canReadJobLogs } from '@/Modules/Jobs/Types/JobStates';
import { readAttempt } from '@/Modules/Jobs/Types/LogLink';

export interface IJobLogSectionProps {
  organizationId: string;
  detail: TJobDetail;
  role: TRole | undefined;
  onEnded: () => void;
}

const AttemptPicker = ({ numbers, value, onChange }: { numbers: readonly number[]; value: number; onChange: (n: number) => void }) => {
  const t = useT();
  return (
    <div role="radiogroup" aria-label={t('jobs.log.attempt')} className="flex flex-wrap gap-1.5">
      {numbers.map((n) => (
        <button
          key={n}
          type="button"
          role="radio"
          aria-checked={n === value}
          onClick={() => {
            onChange(n);
          }}
          className={cx(
            'h-btn-sm cursor-pointer rounded-pill border px-3 text-label-sm whitespace-nowrap transition-colors duration-140',
            n === value ? 'border-line-2 bg-surface-raised text-primary' : 'border-line bg-transparent text-secondary hover:text-primary',
          )}
        >
          {t('jobs.attempts.number', { n })}
        </button>
      ))}
    </div>
  );
};

export const JobLogSection = ({ organizationId, detail, role, onEnded }: IJobLogSectionProps) => {
  const t = useT();
  const [Picked, setPicked] = useState<number | null>(() => readAttempt(window.location.search));
  const numbers = detail.Attempts.map((attempt) => attempt.Number).sort((a, b) => b - a);
  const latest = numbers[0];
  const attempt = Picked !== null && numbers.includes(Picked) ? Picked : latest;
  const stepNames = detail.Spec.Steps.map((step) => step.Name);

  let body;
  if (!canReadJobLogs(role)) body = <EmptyState icon="shield" text={t('jobs.log.forbidden')} />;
  else if (attempt === undefined) body = <EmptyState icon="terminal" text={t('jobs.log.notStarted')} />;
  else {
    body = (
      <div className="grid gap-3 px-4 pb-4">
        {numbers.length > 1 ? (
          <AttemptPicker
            numbers={numbers}
            value={attempt}
            onChange={(n) => {
              setPicked(n === latest ? null : n);
            }}
          />
        ) : null}
        <JobLog
          key={attempt}
          organizationId={organizationId}
          jobId={detail.Job.Id}
          attempt={attempt}
          stepNames={stepNames}
          onEnded={onEnded}
        />
      </div>
    );
  }

  return (
    <Card title={t('jobs.log.title')} icon="terminal">
      {body}
    </Card>
  );
};

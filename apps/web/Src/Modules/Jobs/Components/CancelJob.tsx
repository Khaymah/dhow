import { useEffect, useRef, useState } from 'react';

import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { useT } from '@/I18n/useT';
import type { TJob } from '@/Modules/Jobs/Api/JobsApi';
import { useCancelJob } from '@/Modules/Jobs/Hooks/useJobs';

export interface ICancelJobProps {
  organizationId: string;
  job: TJob;
}

export const CancelJob = ({ organizationId, job }: ICancelJobProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const [Confirming, setConfirming] = useState(false);
  const confirmRef = useRef<HTMLButtonElement>(null);
  const cancel = useCancelJob(organizationId, job.Id);

  useEffect(() => {
    if (Confirming) confirmRef.current?.focus();
  }, [Confirming]);

  const HandleConfirm = () => {
    cancel.mutate(undefined, {
      onSuccess: () => {
        setConfirming(false);
        Push({ Id: `job-canceled-${job.Id}`, Accent: 'live', Title: t('jobs.cancel.done', { name: job.Name }) });
      },
    });
  };

  return (
    <div className="grid gap-2.5 border-t border-line pt-3">
      {Confirming ? <p className="m-0 text-body-sm text-secondary">{t('jobs.cancel.about')}</p> : null}
      {cancel.error ? <ErrorAlert error={cancel.error} /> : null}
      <div className="flex flex-wrap gap-2">
        {Confirming ? (
          <>
            <Button ref={confirmRef} size="sm" variant="danger" iconStart="stop" disabled={cancel.isPending} onClick={HandleConfirm}>
              {t('jobs.cancel.confirm')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={cancel.isPending}
              onClick={() => {
                setConfirming(false);
                cancel.reset();
              }}
            >
              {t('jobs.cancel.back')}
            </Button>
          </>
        ) : (
          <Button
            size="sm"
            variant="danger"
            iconStart="stop"
            onClick={() => {
              setConfirming(true);
            }}
          >
            {t('jobs.cancel.action')}
          </Button>
        )}
      </div>
    </div>
  );
};

import { useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useAction } from '@/Core/Hooks/useAction';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { Field } from '@/Core/Ui/Field';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import { pauseEnvironment, resumeEnvironment, type TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { useEnvironmentRefresh } from '@/Modules/Environments/Hooks/useEnvironmentRefresh';
import { hasInvisible, REASON_MAX } from '@/Modules/Environments/Types/EnvironmentRules';

export interface IPauseCardProps {
  organizationId: string;
  projectId: string;
  environment: TEnvironment;
  canPause: boolean;
  canResume: boolean;
}

interface IPanelProps extends IPauseCardProps {
  methods: readonly TFactorMethod[];
}

const PauseForm = ({ organizationId, projectId, environment }: IPauseCardProps) => {
  const t = useT();
  const refresh = useEnvironmentRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const [Reason, setReason] = useState('');
  const [Problem, setProblem] = useState<string | null>(null);
  const pause = useAction((reason: string) => pauseEnvironment(organizationId, projectId, environment.Name, reason));

  const HandleSubmit = () => {
    const reason = Reason.trim();
    if (reason === '' || reason.length > REASON_MAX || hasInvisible(reason)) {
      setProblem(t('environments.pause.error.reason'));
      return;
    }
    setProblem(null);
    void pause.Run(reason).then((result) => {
      if (isApiError(result)) return;
      refresh.Saved(result);
      Push({ Id: `environment-paused-${String(result.Version)}`, Accent: 'awaiting', Title: t('environments.paused', { name: result.Name }) });
    });
  };

  return (
    <form
      noValidate
      className="grid gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        HandleSubmit();
      }}
    >
      <p className="m-0 text-body-sm text-secondary">{t('environments.pause.about')}</p>
      {Problem ? <InlineAlert accent="failed" title={Problem} /> : null}
      {pause.Failure ? <ErrorAlert error={pause.Failure} /> : null}
      <Field label={t('environments.pause.reason')}>
        <Textarea
          rows={2}
          maxLength={REASON_MAX}
          disabled={pause.Pending}
          value={Reason}
          onChange={(event) => {
            setReason(event.target.value);
          }}
        />
      </Field>
      <Button type="submit" size="sm" variant="primary" iconStart="stop" className="justify-self-start" disabled={pause.Pending}>
        {pause.Pending ? t('environments.pause.submitting') : t('environments.pause.submit')}
      </Button>
    </form>
  );
};

const ResumePanel = ({ organizationId, projectId, environment, methods }: IPanelProps) => {
  const t = useT();
  const refresh = useEnvironmentRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const [Asking, setAsking] = useState(false);
  const action = useFactorAction<TEnvironment>(methods, async (proof) =>
    resumeEnvironment(organizationId, projectId, environment.Name, await proof()),
  );

  if (!Asking) {
    return (
      <Button
        size="sm"
        variant="primary"
        iconStart="play"
        className="justify-self-start"
        onClick={() => {
          setAsking(true);
        }}
      >
        {t('environments.resume')}
      </Button>
    );
  }
  return (
    <div className="-mx-card -mb-card">
      <ConfirmPanel
        question={t('environments.resume.question', { name: environment.Name })}
        confirm={t('environments.resume.confirm')}
        icon="play"
        methods={methods}
        action={action}
        onCancel={() => {
          setAsking(false);
          action.Reset();
        }}
        onConfirm={() => {
          void action.Run().then((resumed) => {
            if (resumed === null) return;
            refresh.Saved(resumed);
            Push({ Id: `environment-resumed-${String(resumed.Version)}`, Accent: 'live', Title: t('environments.resumed', { name: resumed.Name }) });
          });
        }}
      />
    </div>
  );
};

const PausedState = (props: IPauseCardProps) => {
  const t = useT();
  const time = useRelativeTime();
  const { environment, canResume } = props;
  const since = environment.PausedAt === undefined ? '' : time.ago(environment.PausedAt);

  return (
    <div className="grid gap-3">
      <InlineAlert accent="awaiting" title={t('environments.pause.active')}>
        {environment.PausedReason ?? ''} {since === '' ? '' : `· ${since}`}
      </InlineAlert>
      <p className="m-0 text-body-sm text-secondary">{t('environments.pause.running')}</p>
      {canResume ? (
        <FactorGate>{(methods) => <ResumePanel {...props} methods={methods} />}</FactorGate>
      ) : (
        <p className="m-0 text-body-sm text-secondary">{t('environments.resume.forbidden')}</p>
      )}
    </div>
  );
};

export const PauseCard = (props: IPauseCardProps) => {
  const t = useT();
  const { environment, canPause } = props;

  return (
    <Card title={t('environments.pause.title')} icon="stop" padded>
      {environment.Paused ? (
        <PausedState {...props} />
      ) : canPause ? (
        <PauseForm {...props} />
      ) : (
        <p className="m-0 text-body-sm text-secondary">{t('environments.pause.forbidden')}</p>
      )}
    </Card>
  );
};

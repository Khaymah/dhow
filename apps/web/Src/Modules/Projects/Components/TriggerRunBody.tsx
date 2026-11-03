import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { runsKey, triggerRun, type TManualInputs } from '@/Modules/Projects/Api/RunsApi';
import { HintedField } from '@/Modules/Projects/Components/HintedField';
import { InputField } from '@/Modules/Projects/Components/InputField';
import { useIssueText, type TIssues } from '@/Modules/Projects/Hooks/useIssueText';
import { useOrganizationAccess } from '@/Modules/Projects/Hooks/useProjectAccess';
import { useProject } from '@/Modules/Projects/Hooks/useProjects';
import { isNotLinked } from '@/Modules/Projects/Hooks/useRepository';
import { useManualInputs } from '@/Modules/Projects/Hooks/useRuns';
import { EXAMPLES } from '@/Modules/Projects/Types/LinkForm';
import { canOnProject } from '@/Modules/Projects/Types/ProjectAccess';
import { shortRef } from '@/Modules/Projects/Types/RunStates';
import { initialValues, requestInputs, triggerSchema, type TInputValues } from '@/Modules/Projects/Types/TriggerForm';

export interface ITriggerRunBodyProps {
  projectId: string | undefined;
  onStarted: (runId: string) => void;
}

interface IFormProps {
  organizationId: string;
  projectId: string;
  manual: TManualInputs;
  onStarted: (runId: string) => void;
}

const TriggerForm = ({ organizationId, projectId, manual, onStarted }: IFormProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const issueText = useIssueText();
  const client = useQueryClient();
  const [Ref, setRef] = useState(shortRef(manual.Ref));
  const [Sha, setSha] = useState('');
  const [Values, setValues] = useState<TInputValues>(() => initialValues(manual.Inputs));
  const [Problems, setProblems] = useState<TIssues>({});
  const [ServerError, setServerError] = useState<string | null>(null);
  const [Busy, setBusy] = useState(false);

  const HandleSubmit = () => {
    const parsed = triggerSchema(manual.Inputs).safeParse({ Ref, Sha, Inputs: Values });
    if (!parsed.success) {
      setProblems(issueText(parsed.error));
      return;
    }
    setProblems({});
    setServerError(null);
    setBusy(true);
    const inputs = requestInputs(manual.Inputs, Values);
    triggerRun(organizationId, projectId, {
      Ref: parsed.data.Ref,
      ...(parsed.data.Sha === '' ? {} : { Sha: parsed.data.Sha }),
      ...(Object.keys(inputs).length === 0 ? {} : { Inputs: inputs }),
    })
      .then((run) => {
        void client.invalidateQueries({ queryKey: runsKey(organizationId, projectId) });
        onStarted(run.Id);
      })
      .catch((error: unknown) => {
        setServerError(errorText(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <form
      noValidate
      className="grid gap-5"
      onSubmit={(event) => {
        event.preventDefault();
        HandleSubmit();
      }}
    >
      <p className="m-0 text-body-sm text-secondary">{manual.Known ? t('runs.trigger.lead') : t('runs.trigger.unknown')}</p>
      {ServerError === null ? null : <InlineAlert accent="failed" title={ServerError} />}
      <HintedField label={t('runs.trigger.ref')} hint={t('runs.trigger.ref.hint', { branch: EXAMPLES.branch, tag: EXAMPLES.tag })} error={Problems.Ref}>
        <Input dir="ltr" autoComplete="off" spellCheck={false} value={Ref} disabled={Busy} onChange={(event) => { setRef(event.target.value); }} />
      </HintedField>
      <HintedField label={t('runs.trigger.sha')} hint={t('runs.trigger.sha.hint')} error={Problems.Sha}>
        <Input dir="ltr" autoComplete="off" spellCheck={false} value={Sha} disabled={Busy} onChange={(event) => { setSha(event.target.value); }} />
      </HintedField>
      {manual.Inputs.length === 0 ? null : (
        <fieldset className="m-0 grid gap-4 rounded-md border border-line p-4">
          <legend className="px-1 text-label">{t('runs.trigger.inputs')}</legend>
          {manual.Inputs.map((input) => (
            <InputField
              key={input.Name}
              input={input}
              value={Values[input.Name]}
              error={Problems[`Inputs.${input.Name}`]}
              disabled={Busy}
              onChange={(value) => { setValues((current) => ({ ...current, [input.Name]: value })); }}
            />
          ))}
        </fieldset>
      )}
      <Button type="submit" variant="primary" iconStart="play" className="justify-self-start" disabled={Busy}>
        {Busy ? t('runs.trigger.submitting') : t('runs.trigger.submit')}
      </Button>
    </form>
  );
};

const Notice = ({ text }: { text: string }) => (
  <Card>
    <EmptyState icon="play" text={text} />
  </Card>
);

export const TriggerRunBody = ({ projectId, onStarted }: ITriggerRunBodyProps) => {
  const t = useT();
  const access = useOrganizationAccess();
  const organizationId = access.Kind === 'allowed' ? access.OrganizationId : '';
  const project = useProject(organizationId, projectId ?? '');
  const role = project.data?.CallerRole ?? (access.Kind === 'allowed' ? access.Role : undefined);
  const allowed = canOnProject(role, 'run.trigger') && project.data?.State === 'active' && project.data.RepositoryLinked;
  const manual = useManualInputs(organizationId, projectId ?? '', allowed && organizationId !== '' && projectId !== undefined);

  if (projectId === undefined || access.Kind === 'denied') return <Notice text={t('repository.link.missing')} />;
  if (access.Kind === 'loading' || project.isPending) return <Skeleton label={t('ui.loading')} lines={5} />;
  if (project.isError) return <InlineAlert accent="failed" title={t('projects.detail.error')} />;
  if (project.data.State === 'archived') return <Notice text={t('runs.trigger.archived')} />;
  if (!canOnProject(role, 'run.trigger')) return <Notice text={t('runs.trigger.forbidden')} />;
  if (!project.data.RepositoryLinked) return <Notice text={t('runs.trigger.notLinked')} />;
  if (isNotLinked(manual.error)) return <Notice text={t('runs.trigger.notLinked')} />;
  if (manual.isError) return <InlineAlert accent="failed" title={t('runs.trigger.error')} />;
  if (manual.data === undefined) return <Skeleton label={t('ui.loading')} lines={5} />;
  if (manual.data.Known && !manual.data.Enabled) return <Notice text={t('runs.trigger.disabled', { key: EXAMPLES.manualKey })} />;
  return <TriggerForm organizationId={organizationId} projectId={projectId} manual={manual.data} onStarted={onStarted} />;
};

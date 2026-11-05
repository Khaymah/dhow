import { useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Field } from '@/Core/Ui/Field';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { createEnvironment, type TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { FactorGate } from '@/Modules/Environments/Components/FactorGate';
import { FactorSubmit } from '@/Modules/Environments/Components/FactorSubmit';
import { RuleFields } from '@/Modules/Environments/Components/RuleFields';
import { useEnvironmentRefresh } from '@/Modules/Environments/Hooks/useEnvironmentRefresh';
import { useRulesDraft } from '@/Modules/Environments/Hooks/useRulesDraft';
import { NAME_PATTERN } from '@/Modules/Environments/Types/EnvironmentRules';
import { draftProblem, EMPTY_DRAFT, rulesInputOf } from '@/Modules/Environments/Types/RulesDraft';

export interface IEnvironmentNewBodyProps {
  projectId?: string;
  onCreated: (name: string) => void;
}

interface INewFormProps extends IEnvironmentNewBodyProps {
  organizationId: string;
  projectId: string;
  methods: readonly TFactorMethod[];
}

const NewForm = ({ organizationId, projectId, methods, onCreated }: INewFormProps) => {
  const t = useT();
  const refresh = useEnvironmentRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const form = useRulesDraft(EMPTY_DRAFT);
  const [Name, setName] = useState('');
  const [Problem, setProblem] = useState<string | null>(null);
  const [Taken, setTaken] = useState<string | null>(null);

  const action = useFactorAction<TEnvironment>(methods, async (proof) => {
    setTaken(null);
    try {
      const factor = await proof();
      const rules = rulesInputOf(form.Draft, EMPTY_DRAFT, true, true);
      return await createEnvironment(organizationId, projectId, { Name: Name.trim(), Description: form.Draft.Description, Rules: rules }, factor);
    } catch (error) {
      if (isApiError(error) && error.Code === 'ENVIRONMENT_EXISTS') setTaken(t('environments.error.taken'));
      throw error;
    }
  });

  const HandleSubmit = () => {
    const problem = NAME_PATTERN.test(Name.trim()) ? draftProblem(form.Draft, true) : 'environments.error.name';
    setProblem(problem === null ? null : t(problem));
    if (problem !== null) return;
    void action.Run().then((created) => {
      if (created === null) return;
      refresh.Saved(created);
      Push({ Id: `environment-created-${created.Name}`, Accent: 'live', Title: t('environments.created', { name: created.Name }) });
      onCreated(created.Name);
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
      {Problem ? <InlineAlert accent="failed" title={Problem} /> : null}
      {action.Notice ? <InlineAlert accent="failed" title={action.Notice} /> : null}
      <div className="grid gap-1.5">
        <Field label={t('environments.form.name')} error={Taken ?? undefined}>
          <Input
            dir="ltr"
            className="font-mono"
            maxLength={64}
            autoComplete="off"
            spellCheck={false}
            disabled={action.Busy}
            value={Name}
            onChange={(event) => {
              setName(event.target.value);
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('environments.form.name.hint')}</small>
      </div>
      <RuleFields form={form} organizationId={organizationId} disabled={action.Busy} reviewersEditable creating />
      <FactorSubmit
        methods={methods}
        action={action}
        label={t('environments.form.submit')}
        busyLabel={t('environments.form.submitting')}
        icon="plus"
      />
    </form>
  );
};

export const EnvironmentNewBody = ({ projectId, onCreated }: IEnvironmentNewBodyProps) => {
  const t = useT();
  const membership = useCurrentMembership();
  if (membership === undefined) return <Skeleton label={t('ui.loading')} lines={5} />;
  if (projectId === undefined) return <InlineAlert accent="awaiting" title={t('environments.form.noProject')} />;
  return (
    <FactorGate>
      {(methods) => (
        <NewForm organizationId={membership.OrganizationId} projectId={projectId} methods={methods} onCreated={onCreated} />
      )}
    </FactorGate>
  );
};

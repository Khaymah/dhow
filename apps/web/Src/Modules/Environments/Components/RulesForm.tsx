import { useState } from 'react';

import { isApiError, type ApiError } from '@/Core/Api/ApiError';
import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { updateEnvironment, type IUpdateEnvironment, type TEnvironment } from '@/Modules/Environments/Api/EnvironmentsApi';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { FactorSubmit } from '@/Modules/Environments/Components/FactorSubmit';
import { RuleFields } from '@/Modules/Environments/Components/RuleFields';
import { useEnvironmentRefresh } from '@/Modules/Environments/Hooks/useEnvironmentRefresh';
import { useRulesDraft } from '@/Modules/Environments/Hooks/useRulesDraft';
import { draftOf, draftProblem, emptiesRefs, hasDraftChanges, rulesInputOf } from '@/Modules/Environments/Types/RulesDraft';

export interface IRulesFormProps {
  organizationId: string;
  projectId: string;
  environment: TEnvironment;
  onReload: () => void;
}

interface IEditableRulesProps extends IRulesFormProps {
  methods: readonly TFactorMethod[];
}

const FailureNotes = ({ failure, notice, onReload }: { failure: ApiError | null; notice: string | null; onReload: () => void }) => {
  const t = useT();
  if (notice === null) return null;
  const details = failure?.Code === 'REVIEWER_NOT_ELIGIBLE' ? failure.Details.map((detail) => detail.Message).join(' ') : '';
  return (
    <div className="grid justify-items-start gap-2">
      <InlineAlert accent="failed" title={notice}>
        {details === '' ? null : details}
      </InlineAlert>
      {failure?.Code === 'ENVIRONMENT_VERSION_CONFLICT' ? (
        <Button size="sm" iconStart="swap" onClick={onReload}>
          {t('environments.conflict.reload')}
        </Button>
      ) : null}
    </div>
  );
};

const EditableRules = ({ organizationId, projectId, environment, methods, onReload }: IEditableRulesProps) => {
  const t = useT();
  const refresh = useEnvironmentRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const original = draftOf(environment);
  const form = useRulesDraft(original);
  const reviewersEditable = environment.Rules.Reviewers.IdsShown;
  const [Failure, setFailure] = useState<ApiError | null>(null);
  const [Problem, setProblem] = useState<string | null>(null);
  const [EmptyWarned, setEmptyWarned] = useState(false);
  const changed = hasDraftChanges(form.Draft, original, reviewersEditable);

  const action = useFactorAction<TEnvironment>(methods, async (proof) => {
    setFailure(null);
    try {
      const factor = await proof();
      const rules = rulesInputOf(form.Draft, original, false, reviewersEditable);
      const body: IUpdateEnvironment = {
        Version: environment.Version,
        ...(form.Draft.Description === original.Description ? {} : { Description: form.Draft.Description }),
        ...(Object.keys(rules).length === 0 ? {} : { Rules: rules }),
      };
      return await updateEnvironment(organizationId, projectId, environment.Name, body, factor);
    } catch (error) {
      if (isApiError(error)) setFailure(error);
      throw error;
    }
  });

  const HandleSubmit = () => {
    const problem = draftProblem(form.Draft, reviewersEditable);
    setProblem(problem === null ? null : t(problem));
    if (problem !== null) return;
    if (emptiesRefs(form.Draft, original) && !EmptyWarned) {
      setEmptyWarned(true);
      return;
    }
    void action.Run().then((saved) => {
      if (saved === null) return;
      refresh.Saved(saved);
      Push({ Id: `environment-saved-${String(saved.Version)}`, Accent: 'live', Title: t('environments.saved', { name: saved.Name }) });
    });
  };

  return (
    <Card title={t('environments.rules.title')} icon="shield" padded>
      <form
        noValidate
        className="grid gap-5"
        onSubmit={(event) => {
          event.preventDefault();
          HandleSubmit();
        }}
      >
        {Problem ? <InlineAlert accent="failed" title={Problem} /> : null}
        {EmptyWarned && emptiesRefs(form.Draft, original) ? <InlineAlert accent="awaiting" title={t('environments.refs.emptyConfirm')} /> : null}
        <FailureNotes failure={Failure} notice={action.Notice} onReload={onReload} />
        <RuleFields form={form} organizationId={organizationId} disabled={action.Busy} reviewersEditable={reviewersEditable} creating={false} />
        {reviewersEditable ? null : <InlineAlert accent="building" title={t('environments.reviewers.hidden')} />}
        <FactorSubmit
          methods={methods}
          action={action}
          label={t('environments.form.save')}
          busyLabel={t('environments.form.saving')}
          icon="check"
          disabled={!changed}
        />
      </form>
    </Card>
  );
};

export const RulesForm = (props: IRulesFormProps) => (
  <FactorGate>{(methods) => <EditableRules key={props.environment.Version} {...props} methods={methods} />}</FactorGate>
);

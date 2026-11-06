import { useState } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { updateTarget, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { FactorSubmit } from '@/Modules/Environments/Components/FactorSubmit';
import { TargetFields } from '@/Modules/Environments/Components/TargetFields';
import { useTargetDraft } from '@/Modules/Environments/Hooks/useTargetDraft';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';
import { draftOfTarget, draftProblem, updateInputOf } from '@/Modules/Environments/Types/TargetDraft';

export interface ITargetEditFormProps {
  scope: ITargetScope;
  target: TTarget;
}

const Form = ({ scope, target, methods }: ITargetEditFormProps & { methods: readonly TFactorMethod[] }) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const Push = useToastStore((state) => state.Push);
  const form = useTargetDraft(draftOfTarget(target));
  const [Problem, setProblem] = useState<string | null>(null);
  const action = useFactorAction<TTarget>(methods, async (proof) =>
    updateTarget(scope, target.Name, target.Version, updateInputOf(form.Draft), await proof()),
  );

  const HandleSubmit = () => {
    const problem = draftProblem(form.Draft, false);
    setProblem(problem === null ? null : t(problem));
    if (problem !== null) return;
    void action.Run().then((saved) => {
      if (saved === null) return;
      refresh.Changed();
      Push({ Id: `target-saved-${saved.Name}-${String(saved.Version)}`, Accent: 'live', Title: t('targets.saved', { name: saved.Name }) });
    });
  };

  return (
    <form
      noValidate
      className="grid gap-5 p-card"
      onSubmit={(event) => {
        event.preventDefault();
        HandleSubmit();
      }}
    >
      {Problem ? <InlineAlert focus accent="failed" title={Problem} /> : null}
      {action.Notice ? <InlineAlert focus accent="failed" title={action.Notice} /> : null}
      <TargetFields form={form} creating={false} disabled={action.Busy} />
      <FactorSubmit methods={methods} action={action} label={t('targets.form.save')} busyLabel={t('targets.form.saving')} icon="check" />
    </form>
  );
};

export const TargetEditForm = (props: ITargetEditFormProps) => (
  <FactorGate>{(methods) => <Form {...props} methods={methods} />}</FactorGate>
);

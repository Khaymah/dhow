import { useState } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { createTarget, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { FactorSubmit } from '@/Modules/Environments/Components/FactorSubmit';
import { HostRowsField } from '@/Modules/Environments/Components/HostRowsField';
import { TargetFields } from '@/Modules/Environments/Components/TargetFields';
import { useTargetDraft } from '@/Modules/Environments/Hooks/useTargetDraft';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';
import { createInputOf, draftProblem, EMPTY_TARGET_DRAFT } from '@/Modules/Environments/Types/TargetDraft';

export interface ITargetCreateFormProps {
  scope: ITargetScope;
  onCreated: (name: string) => void;
}

const Form = ({ scope, methods, onCreated }: ITargetCreateFormProps & { methods: readonly TFactorMethod[] }) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const Push = useToastStore((state) => state.Push);
  const form = useTargetDraft(EMPTY_TARGET_DRAFT);
  const [Problem, setProblem] = useState<string | null>(null);
  const action = useFactorAction<TTarget>(methods, async (proof) =>
    createTarget(scope, createInputOf(form.Draft), await proof()),
  );

  const HandleSubmit = () => {
    const problem = draftProblem(form.Draft, true);
    setProblem(problem === null ? null : t(problem));
    if (problem !== null) return;
    void action.Run().then((created) => {
      if (created === null) return;
      refresh.Changed();
      Push({ Id: `target-created-${created.Name}`, Accent: 'live', Title: t('targets.created', { name: created.Name }) });
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
      <TargetFields form={form} creating disabled={action.Busy} />
      <HostRowsField form={form} disabled={action.Busy} />
      <InlineAlert accent="building" title={t('targets.form.afterCreate')} />
      <FactorSubmit methods={methods} action={action} label={t('targets.form.create')} busyLabel={t('targets.form.creating')} icon="plus" />
    </form>
  );
};

export const TargetCreateForm = (props: ITargetCreateFormProps) => (
  <FactorGate>{(methods) => <Form {...props} methods={methods} />}</FactorGate>
);

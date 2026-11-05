import { useState } from 'react';

import { Button } from '@/Core/Ui/Button';
import { Field } from '@/Core/Ui/Field';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import { putVariable, type ISecretScope, type TVariable } from '@/Modules/Secrets/Api/SecretsApi';
import { NameField } from '@/Modules/Secrets/Components/NameField';
import { FormGate, FormLookup } from '@/Modules/Secrets/Components/FormGate';
import { useSecretSave } from '@/Modules/Secrets/Hooks/useSecretSave';
import { useVariables } from '@/Modules/Secrets/Hooks/useSecretsData';
import { byteLength, NAME_PATTERN, VARIABLE_MAX_BYTES } from '@/Modules/Secrets/Types/SecretRules';

export interface IVariableFormBodyProps {
  projectId?: string;
  name?: string;
  onDone: () => void;
}

interface IVariableFormProps {
  scope: ISecretScope;
  existing: TVariable | undefined;
  onDone: () => void;
}

const VariableForm = ({ scope, existing, onDone }: IVariableFormProps) => {
  const t = useT();
  const save = useSecretSave(onDone);
  const [Name, setName] = useState('');
  const [Value, setValue] = useState(existing?.Value ?? '');
  const editing = existing !== undefined;
  const variableName = existing?.Name ?? Name.trim();

  const HandleSubmit = () => {
    const problem = !NAME_PATTERN.test(variableName)
      ? t('secrets.form.error.name')
      : byteLength(Value) > VARIABLE_MAX_BYTES
        ? t('variables.form.error.large')
        : null;
    save.SetNotice(problem);
    if (problem !== null) return;
    save.Save(
      () => putVariable(scope, variableName, Value),
      t('variables.saved', { name: variableName }),
      () => undefined,
    );
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
      {save.Notice ? <InlineAlert accent="failed" title={save.Notice} /> : null}
      {editing ? null : <NameField label={t('variables.form.name')} hint={t('variables.form.name.hint')} value={Name} onChange={setName} />}
      <div className="grid gap-1.5">
        <Field label={t('variables.form.value')}>
          <Textarea
            dir="ltr"
            rows={4}
            className="font-mono"
            autoComplete="off"
            spellCheck={false}
            disabled={save.Busy}
            value={Value}
            onChange={(event) => {
              setValue(event.target.value);
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('variables.form.value.hint')}</small>
      </div>
      <Button type="submit" variant="primary" iconStart="check" className="justify-self-start" disabled={save.Busy}>
        {save.Busy ? t('secrets.form.submitting') : t('variables.form.submit')}
      </Button>
    </form>
  );
};

const ExistingVariable = ({ scope, name, onDone }: { scope: ISecretScope; name: string; onDone: () => void }) => {
  const t = useT();
  const query = useVariables(scope);
  return (
    <FormLookup query={query} find={(items) => items.find((item) => item.Name === name)} missing={t('variables.form.notFound')}>
      {(variable) => <VariableForm key={variable.Name} scope={scope} existing={variable} onDone={onDone} />}
    </FormLookup>
  );
};

export const VariableFormBody = ({ projectId, name, onDone }: IVariableFormBodyProps) => (
  <FormGate projectId={projectId}>
    {(scope) =>
      name === undefined ? (
        <VariableForm key={scope.OrganizationId} scope={scope} existing={undefined} onDone={onDone} />
      ) : (
        <ExistingVariable scope={scope} name={name} onDone={onDone} />
      )
    }
  </FormGate>
);

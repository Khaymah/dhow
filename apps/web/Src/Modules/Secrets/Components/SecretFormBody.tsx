import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import { putSecret, type ISecretScope, type TSecret } from '@/Modules/Secrets/Api/SecretsApi';
import { FormGate, FormLookup } from '@/Modules/Secrets/Components/FormGate';
import { SecretFields } from '@/Modules/Secrets/Components/SecretFields';
import { SecretRemoveZone } from '@/Modules/Secrets/Components/SecretRemoveZone';
import { useSecretDraft, type ISecretDraft } from '@/Modules/Secrets/Hooks/useSecretDraft';
import { useSecretSave } from '@/Modules/Secrets/Hooks/useSecretSave';
import { useSecrets } from '@/Modules/Secrets/Hooks/useSecretsData';
import { DESCRIPTION_MAX, nameProblem, secretValueProblem } from '@/Modules/Secrets/Types/SecretRules';
import type { TTranslate } from '@/I18n/I18nContext';

export interface ISecretFormBodyProps {
  projectId?: string;
  environmentName?: string;
  name?: string;
  onDone: () => void;
}

interface ISecretFormProps {
  scope: ISecretScope;
  existing: TSecret | undefined;
  onDone: () => void;
}

const draftProblem = (t: TTranslate, draft: ISecretDraft, name: string, editing: boolean, organization: boolean): string | null => {
  const problem = nameProblem(name);
  if (!editing && problem === 'reserved') return t('secrets.form.error.reserved');
  if (!editing && problem !== null) return t('secrets.form.error.name');
  const value = secretValueProblem(draft.Value);
  if (value !== null) return t(`secrets.form.error.${value}`);
  if (draft.Description.length > DESCRIPTION_MAX) return t('secrets.form.error.description');
  if (organization && draft.Visibility === 'selected_projects' && draft.ProjectIds.length === 0) return t('secrets.form.error.projects');
  return null;
};

const SecretForm = ({ scope, existing, onDone }: ISecretFormProps) => {
  const t = useT();
  const save = useSecretSave(onDone);
  const draft = useSecretDraft(existing);
  const editing = existing !== undefined;
  const organization = scope.ProjectId === null;
  const secretName = existing?.Name ?? draft.Name.trim();

  const HandleSubmit = () => {
    const problem = draftProblem(t, draft, secretName, editing, organization);
    save.SetNotice(problem);
    if (problem !== null) return;
    save.Save(
      () =>
        putSecret(scope, secretName, {
          Value: draft.Value,
          Description: draft.Description,
          AvailableToPullRequests: draft.Pulls,
          Visibility: draft.Visibility,
          ProjectIds: draft.ProjectIds,
        }),
      t('secrets.saved', { name: secretName }),
      () => {
        draft.SetValue('');
      },
    );
  };

  return (
    <div className="grid gap-5">
      <form
        noValidate
        className="grid gap-5"
        onSubmit={(event) => {
          event.preventDefault();
          HandleSubmit();
        }}
      >
        {save.Notice ? <InlineAlert accent="failed" title={save.Notice} /> : null}
        <SecretFields
          draft={draft}
          editing={editing}
          organizationId={organization ? scope.OrganizationId : null}
          pullRequests={scope.EnvironmentName === undefined}
          busy={save.Busy}
        />
        <Button type="submit" variant="primary" iconStart="check" className="justify-self-start" disabled={save.Busy}>
          {save.Busy ? t('secrets.form.submitting') : editing ? t('secrets.form.submit.edit') : t('secrets.form.submit')}
        </Button>
      </form>
      {existing ? <SecretRemoveZone scope={scope} name={existing.Name} disabled={save.Busy} onRemoved={onDone} /> : null}
    </div>
  );
};

const ExistingSecret = ({ scope, name, onDone }: { scope: ISecretScope; name: string; onDone: () => void }) => {
  const t = useT();
  const query = useSecrets(scope);
  return (
    <FormLookup query={query} find={(items) => items.find((item) => item.Name === name)} missing={t('secrets.form.notFound')}>
      {(secret) => <SecretForm key={secret.Name} scope={scope} existing={secret} onDone={onDone} />}
    </FormLookup>
  );
};

export const SecretFormBody = ({ projectId, environmentName, name, onDone }: ISecretFormBodyProps) => (
  <FormGate projectId={projectId} environmentName={environmentName}>
    {(scope) =>
      name === undefined ? (
        <SecretForm key={scope.OrganizationId} scope={scope} existing={undefined} onDone={onDone} />
      ) : (
        <ExistingSecret scope={scope} name={name} onDone={onDone} />
      )
    }
  </FormGate>
);

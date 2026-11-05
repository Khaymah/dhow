import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import { NameField } from '@/Modules/Secrets/Components/NameField';
import { PullRequestsToggle } from '@/Modules/Secrets/Components/PullRequestsToggle';
import { SecretVisibilityFields } from '@/Modules/Secrets/Components/SecretVisibilityFields';
import type { ISecretDraft } from '@/Modules/Secrets/Hooks/useSecretDraft';
import { DESCRIPTION_MAX } from '@/Modules/Secrets/Types/SecretRules';

export interface ISecretFieldsProps {
  draft: ISecretDraft;
  editing: boolean;
  organizationId: string | null;
  busy: boolean;
}

export const SecretFields = ({ draft, editing, organizationId, busy }: ISecretFieldsProps) => {
  const t = useT();

  return (
    <>
      {editing ? null : (
        <NameField label={t('secrets.form.name')} hint={t('secrets.form.name.hint')} value={draft.Name} onChange={draft.SetName} />
      )}
      <div className="grid gap-1.5">
        <Field label={editing ? t('secrets.form.value.edit') : t('secrets.form.value')}>
          <Textarea
            dir="ltr"
            rows={5}
            className="font-mono"
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            disabled={busy}
            value={draft.Value}
            onChange={(event) => {
              draft.SetValue(event.target.value);
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('secrets.form.value.hint')}</small>
      </div>
      <div className="grid gap-1.5">
        <Field label={t('secrets.form.description')}>
          <Input
            maxLength={DESCRIPTION_MAX}
            autoComplete="off"
            value={draft.Description}
            onChange={(event) => {
              draft.SetDescription(event.target.value);
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('secrets.form.description.hint')}</small>
      </div>
      <PullRequestsToggle checked={draft.Pulls} disabled={busy} onChange={draft.SetPulls} />
      {organizationId === null ? null : (
        <SecretVisibilityFields
          organizationId={organizationId}
          visibility={draft.Visibility}
          projectIds={draft.ProjectIds}
          disabled={busy}
          onVisibility={draft.SetVisibility}
          onProjectIds={draft.SetProjectIds}
        />
      )}
    </>
  );
};

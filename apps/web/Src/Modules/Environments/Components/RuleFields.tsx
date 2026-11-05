import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import { CheckField } from '@/Modules/Environments/Components/CheckField';
import { RefsField } from '@/Modules/Environments/Components/RefsField';
import { ReviewersField } from '@/Modules/Environments/Components/ReviewersField';
import type { IRulesForm } from '@/Modules/Environments/Hooks/useRulesDraft';
import { DESCRIPTION_MAX, MAX_PLUGINS, TIMEOUT_MAX, TIMEOUT_MIN, WAIT_MAX } from '@/Modules/Environments/Types/EnvironmentRules';

export interface IRuleFieldsProps {
  form: IRulesForm;
  organizationId: string;
  disabled: boolean;
  reviewersEditable: boolean;
  creating: boolean;
}

const NumberField = ({
  label,
  hint,
  value,
  disabled,
  onChange,
}: {
  label: string;
  hint: string;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) => (
  <div className="grid gap-1.5">
    <Field label={label}>
      <Input
        dir="ltr"
        inputMode="numeric"
        autoComplete="off"
        maxLength={6}
        disabled={disabled}
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
        }}
      />
    </Field>
    <small className="text-body-sm text-tertiary">{hint}</small>
  </div>
);

const RuleBasics = ({ form, organizationId, disabled, reviewersEditable, creating }: IRuleFieldsProps) => {
  const t = useT();
  const { Draft, Patch } = form;

  return (
    <>
      <div className="grid gap-1.5">
        <Field label={t('environments.form.description')}>
          <Textarea
            rows={2}
            maxLength={DESCRIPTION_MAX}
            disabled={disabled}
            value={Draft.Description}
            onChange={(event) => {
              Patch({ Description: event.target.value });
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('environments.form.description.hint')}</small>
      </div>
      <RefsField
        refs={Draft.Refs}
        disabled={disabled}
        fallbackHint={creating && !Draft.RefsEdited}
        onChange={(refs) => {
          Patch({ Refs: refs, RefsEdited: true });
        }}
      />
      <CheckField
        label={t('environments.rule.pullRequests')}
        about={t('environments.rule.pullRequests.about')}
        checked={Draft.AllowPullRequests}
        disabled={disabled}
        onChange={(checked) => {
          Patch({ AllowPullRequests: checked });
        }}
      />
      {reviewersEditable ? (
        <ReviewersField
          organizationId={organizationId}
          userIds={Draft.UserIds}
          teamIds={Draft.TeamIds}
          disabled={disabled}
          onUsers={(ids) => {
            Patch({ UserIds: ids });
          }}
          onTeams={(ids) => {
            Patch({ TeamIds: ids });
          }}
        />
      ) : null}
    </>
  );
};

const RuleLimits = ({ form, disabled }: Pick<IRuleFieldsProps, 'form' | 'disabled'>) => {
  const t = useT();
  const { Draft, Patch } = form;

  return (
    <>
      <CheckField
        label={t('environments.rule.selfApproval')}
        about={t('environments.rule.selfApproval.about')}
        checked={Draft.AllowSelfApproval}
        disabled={disabled}
        onChange={(checked) => {
          Patch({ AllowSelfApproval: checked });
        }}
      />
      <NumberField
        label={t('environments.rule.wait')}
        hint={t('environments.rule.wait.about', { max: WAIT_MAX })}
        value={Draft.WaitMinutes}
        disabled={disabled}
        onChange={(value) => {
          Patch({ WaitMinutes: value });
        }}
      />
      <NumberField
        label={t('environments.rule.timeout')}
        hint={t('environments.rule.timeout.about', { min: TIMEOUT_MIN, max: TIMEOUT_MAX })}
        value={Draft.ApprovalTimeoutHours}
        disabled={disabled}
        onChange={(value) => {
          Patch({ ApprovalTimeoutHours: value });
        }}
      />
      <CheckField
        label={t('environments.rule.pinned')}
        about={t('environments.rule.pinned.about')}
        checked={Draft.RequirePinnedImages}
        disabled={disabled}
        onChange={(checked) => {
          Patch({ RequirePinnedImages: checked });
        }}
      />
      <div className="grid gap-1.5">
        <Field label={t('environments.rule.plugins')}>
          <Textarea
            dir="ltr"
            rows={3}
            className="font-mono"
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
            value={Draft.Plugins}
            onChange={(event) => {
              Patch({ Plugins: event.target.value });
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('environments.rule.plugins.about', { max: MAX_PLUGINS })}</small>
      </div>
      <CheckField
        label={t('environments.rule.bypass')}
        about={t('environments.rule.bypass.about')}
        checked={Draft.AllowAdminBypass}
        disabled={disabled}
        onChange={(checked) => {
          Patch({ AllowAdminBypass: checked });
        }}
      />
      <CheckField
        label={t('environments.rule.supersede')}
        about={t('environments.rule.supersede.about')}
        checked={Draft.SupersedePending}
        disabled={disabled}
        onChange={(checked) => {
          Patch({ SupersedePending: checked });
        }}
      />
    </>
  );
};

export const RuleFields = (props: IRuleFieldsProps) => (
  <>
    <RuleBasics {...props} />
    <RuleLimits form={props.form} disabled={props.disabled} />
  </>
);

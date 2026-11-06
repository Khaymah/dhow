import { Field } from '@/Core/Ui/Field';
import { Input } from '@/Core/Ui/Input';
import { Select } from '@/Core/Ui/Select';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import { CheckField } from '@/Modules/Environments/Components/CheckField';
import type { ITargetForm } from '@/Modules/Environments/Hooks/useTargetDraft';
import type { TTargetKind } from '@/Modules/Environments/Types/TargetDraft';

export interface ITargetFieldsProps {
  form: ITargetForm;
  creating: boolean;
  disabled: boolean;
}

const KINDS: readonly TTargetKind[] = ['ssh', 'compose'];

const TextField = ({
  label,
  hint,
  value,
  disabled,
  mono = true,
  maxLength,
  onChange,
}: {
  label: string;
  hint?: string;
  value: string;
  disabled: boolean;
  mono?: boolean;
  maxLength: number;
  onChange: (value: string) => void;
}) => (
  <div className="grid gap-1.5">
    <Field label={label}>
      <Input
        dir={mono ? 'ltr' : undefined}
        className={mono ? 'font-mono' : undefined}
        maxLength={maxLength}
        autoComplete="off"
        spellCheck={false}
        disabled={disabled}
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
        }}
      />
    </Field>
    {hint ? <small className="text-body-sm text-tertiary">{hint}</small> : null}
  </div>
);

const ListField = ({
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
      <Textarea
        dir="ltr"
        rows={3}
        className="font-mono"
        spellCheck={false}
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

const RollingFields = ({ form, disabled }: { form: ITargetForm; disabled: boolean }) => {
  const t = useT();
  const { Draft, Patch } = form;
  if (Draft.Hosts.length < 2) return null;
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <TextField
        label={t('targets.form.batch')}
        hint={t('targets.form.batch.hint')}
        value={Draft.BatchSize}
        disabled={disabled}
        maxLength={2}
        onChange={(BatchSize) => {
          Patch({ BatchSize });
        }}
      />
      <TextField
        label={t('targets.form.pause')}
        hint={t('targets.form.pause.hint')}
        value={Draft.BatchPauseSeconds}
        disabled={disabled}
        maxLength={3}
        onChange={(BatchPauseSeconds) => {
          Patch({ BatchPauseSeconds });
        }}
      />
    </div>
  );
};

const SshFields = ({ form, disabled }: { form: ITargetForm; disabled: boolean }) => {
  const t = useT();
  const { Draft, Patch } = form;
  return (
    <>
      <TextField
        label={t('targets.form.reload')}
        hint={t('targets.form.reload.hint')}
        value={Draft.ReloadCommand}
        disabled={disabled}
        maxLength={1024}
        onChange={(ReloadCommand) => {
          Patch({ ReloadCommand });
        }}
      />
      <TextField
        label={t('targets.form.preSwitch')}
        value={Draft.PreSwitchCommand}
        disabled={disabled}
        maxLength={1024}
        onChange={(PreSwitchCommand) => {
          Patch({ PreSwitchCommand });
        }}
      />
      <TextField
        label={t('targets.form.postSwitch')}
        value={Draft.PostSwitchCommand}
        disabled={disabled}
        maxLength={1024}
        onChange={(PostSwitchCommand) => {
          Patch({ PostSwitchCommand });
        }}
      />
      <ListField
        label={t('targets.form.shared')}
        hint={t('targets.form.shared.hint')}
        value={Draft.Shared}
        disabled={disabled}
        onChange={(Shared) => {
          Patch({ Shared });
        }}
      />
    </>
  );
};

const ComposeFields = ({ form, disabled }: { form: ITargetForm; disabled: boolean }) => {
  const t = useT();
  const { Draft, Patch } = form;
  return (
    <>
      <TextField
        label={t('targets.form.projectName')}
        hint={t('targets.form.projectName.hint')}
        value={Draft.ProjectName}
        disabled={disabled}
        maxLength={63}
        onChange={(ProjectName) => {
          Patch({ ProjectName });
        }}
      />
      <TextField
        label={t('targets.form.composeFile')}
        hint={t('targets.form.composeFile.hint')}
        value={Draft.ComposeFile}
        disabled={disabled}
        maxLength={100}
        onChange={(ComposeFile) => {
          Patch({ ComposeFile });
        }}
      />
      <CheckField
        label={t('targets.form.unsafe')}
        about={t('targets.form.unsafe.about')}
        checked={Draft.AllowUnsafeCompose}
        disabled={disabled}
        onChange={(AllowUnsafeCompose) => {
          Patch({ AllowUnsafeCompose });
        }}
      />
    </>
  );
};

export const TargetFields = ({ form, creating, disabled }: ITargetFieldsProps) => {
  const t = useT();
  const { Draft, Patch } = form;

  return (
    <div className="grid gap-5">
      {creating ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            label={t('targets.form.name')}
            hint={t('targets.form.name.hint')}
            value={Draft.Name}
            disabled={disabled}
            maxLength={64}
            onChange={(Name) => {
              Patch({ Name });
            }}
          />
          <div className="grid gap-1.5">
            <Field label={t('targets.form.kind')}>
              <Select
                disabled={disabled}
                value={Draft.Kind}
                options={KINDS.map((kind) => ({ value: kind, label: t(`targets.kind.${kind}`) }))}
                onChange={(event) => {
                  Patch({ Kind: event.target.value === 'compose' ? 'compose' : 'ssh' });
                }}
              />
            </Field>
          </div>
        </div>
      ) : null}
      <TextField
        label={t('targets.form.username')}
        hint={t('targets.form.username.hint')}
        value={Draft.Username}
        disabled={disabled}
        maxLength={32}
        onChange={(Username) => {
          Patch({ Username });
        }}
      />
      <CheckField
        label={t('targets.form.root')}
        about={t('targets.form.root.about')}
        checked={Draft.AllowRoot}
        disabled={disabled}
        onChange={(AllowRoot) => {
          Patch({ AllowRoot });
        }}
      />
      <TextField
        label={t('targets.form.path')}
        hint={t('targets.form.path.hint')}
        value={Draft.DeployPath}
        disabled={disabled}
        maxLength={201}
        onChange={(DeployPath) => {
          Patch({ DeployPath });
        }}
      />
      <TextField
        label={t('targets.form.keep')}
        hint={t('targets.form.keep.hint')}
        value={Draft.KeepReleases}
        disabled={disabled}
        maxLength={2}
        onChange={(KeepReleases) => {
          Patch({ KeepReleases });
        }}
      />
      <RollingFields form={form} disabled={disabled} />
      {Draft.Kind === 'ssh' ? <SshFields form={form} disabled={disabled} /> : <ComposeFields form={form} disabled={disabled} />}
      <ListField
        label={t('targets.form.images')}
        hint={t('targets.form.images.hint')}
        value={Draft.AllowedImages}
        disabled={disabled}
        onChange={(AllowedImages) => {
          Patch({ AllowedImages });
        }}
      />
      {creating ? (
        <TextField
          label={t('targets.form.keySecret')}
          hint={t('targets.form.keySecret.hint')}
          value={Draft.KeySecretName}
          disabled={disabled}
          maxLength={64}
          onChange={(KeySecretName) => {
            Patch({ KeySecretName });
          }}
        />
      ) : null}
    </div>
  );
};

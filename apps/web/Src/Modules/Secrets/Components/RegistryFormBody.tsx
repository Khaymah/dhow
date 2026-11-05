import { useState } from 'react';

import { Button } from '@/Core/Ui/Button';
import { Field } from '@/Core/Ui/Field';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { PasswordInput } from '@/Core/Ui/PasswordInput';
import { useT } from '@/I18n/useT';
import {
  createRegistryCredential,
  updateRegistryCredential,
  type ISecretScope,
  type TRegistryCredential,
} from '@/Modules/Secrets/Api/SecretsApi';
import { FormGate, FormLookup } from '@/Modules/Secrets/Components/FormGate';
import { useSecretSave } from '@/Modules/Secrets/Hooks/useSecretSave';
import { useRegistryCredentials } from '@/Modules/Secrets/Hooks/useSecretsData';
import { HOST_MAX, USERNAME_MAX } from '@/Modules/Secrets/Types/SecretRules';

export interface IRegistryFormBodyProps {
  projectId?: string;
  credentialId?: string;
  onDone: () => void;
}

interface IRegistryFormProps {
  scope: ISecretScope;
  existing: TRegistryCredential | undefined;
  onDone: () => void;
}

interface IRegistryFieldsProps {
  existing: TRegistryCredential | undefined;
  host: string;
  onHost: (value: string) => void;
  username: string;
  onUsername: (value: string) => void;
  password: string;
  onPassword: (value: string) => void;
  busy: boolean;
}

const RegistryFields = ({ existing, host, onHost, username, onUsername, password, onPassword, busy }: IRegistryFieldsProps) => {
  const t = useT();
  return (
    <>
      {existing ? (
        <p className="m-0 text-label">
          <bdi dir="ltr" className="font-mono">
            {existing.Host}
          </bdi>
        </p>
      ) : (
        <div className="grid gap-1.5">
          <Field label={t('registry.form.host')}>
            <Input
              dir="ltr"
              className="font-mono"
              maxLength={HOST_MAX}
              autoComplete="off"
              spellCheck={false}
              value={host}
              onChange={(event) => {
                onHost(event.target.value);
              }}
            />
          </Field>
          <small className="text-body-sm text-tertiary">{t('registry.form.host.hint')}</small>
        </div>
      )}
      <Field label={t('registry.form.username')}>
        <Input
          dir="ltr"
          maxLength={USERNAME_MAX}
          autoComplete="off"
          spellCheck={false}
          value={username}
          onChange={(event) => {
            onUsername(event.target.value);
          }}
        />
      </Field>
      <div className="grid gap-1.5">
        <Field label={t('registry.form.password')}>
          <PasswordInput
            autoComplete="new-password"
            disabled={busy}
            value={password}
            onChange={(event) => {
              onPassword(event.target.value);
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('registry.form.password.hint')}</small>
      </div>
    </>
  );
};

const RegistryForm = ({ scope, existing, onDone }: IRegistryFormProps) => {
  const t = useT();
  const save = useSecretSave(onDone);
  const [Host, setHost] = useState('');
  const [Username, setUsername] = useState(existing?.Username ?? '');
  const [Password, setPassword] = useState('');
  const host = existing?.Host ?? Host.trim();

  const HandleSubmit = () => {
    const problem =
      host === ''
        ? t('registry.form.error.host')
        : Username.trim() === ''
          ? t('registry.form.error.username')
          : Password === ''
            ? t('registry.form.error.password')
            : null;
    save.SetNotice(problem);
    if (problem !== null) return;
    const input = { Username: Username.trim(), Password };
    save.Save(
      () => (existing ? updateRegistryCredential(scope, existing.Id, input) : createRegistryCredential(scope, host, input)),
      t('registry.saved', { host }),
      () => {
        setPassword('');
      },
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
      <RegistryFields
        existing={existing}
        host={Host}
        onHost={setHost}
        username={Username}
        onUsername={setUsername}
        password={Password}
        onPassword={setPassword}
        busy={save.Busy}
      />
      <Button type="submit" variant="primary" iconStart="check" className="justify-self-start" disabled={save.Busy}>
        {save.Busy ? t('secrets.form.submitting') : t('registry.form.submit')}
      </Button>
    </form>
  );
};

const ExistingCredential = ({ scope, credentialId, onDone }: { scope: ISecretScope; credentialId: string; onDone: () => void }) => {
  const t = useT();
  const query = useRegistryCredentials(scope);
  return (
    <FormLookup query={query} find={(items) => items.find((item) => item.Id === credentialId)} missing={t('registry.form.notFound')}>
      {(credential) => <RegistryForm key={credential.Id} scope={scope} existing={credential} onDone={onDone} />}
    </FormLookup>
  );
};

export const RegistryFormBody = ({ projectId, credentialId, onDone }: IRegistryFormBodyProps) => (
  <FormGate projectId={projectId}>
    {(scope) =>
      credentialId === undefined ? (
        <RegistryForm key={scope.OrganizationId} scope={scope} existing={undefined} onDone={onDone} />
      ) : (
        <ExistingCredential scope={scope} credentialId={credentialId} onDone={onDone} />
      )
    }
  </FormGate>
);

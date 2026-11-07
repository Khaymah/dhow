import { useRef, useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import { useAction } from '@/Core/Hooks/useAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Field } from '@/Core/Ui/Field';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { Input } from '@/Core/Ui/Input';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { putTargetKeySecret, updateTarget, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { TargetFactorPanel } from '@/Modules/Environments/Components/TargetFactorPanel';
import { useClearOnUnmount } from '@/Modules/Environments/Hooks/useClearOnUnmount';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';
import { defaultKeySecretName, SECRET_NAME_PATTERN } from '@/Modules/Environments/Types/TargetDraft';

export interface IKeySecretFormProps {
  scope: ITargetScope;
  target: TTarget;
}

export const KeySecretForm = ({ scope, target }: IKeySecretFormProps) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const Push = useToastStore((state) => state.Push);
  const valueRef = useRef<HTMLTextAreaElement>(null);
  const [Name, setName] = useState(target.KeySecretName ?? defaultKeySecretName(target.Name));
  const [Empty, setEmpty] = useState(true);
  const [Attach, setAttach] = useState<string | null>(null);
  const nameFixed = target.KeySecretName !== undefined && target.KeySecretName !== '';
  const nameOk = SECRET_NAME_PATTERN.test(Name);
  useClearOnUnmount(valueRef);
  const save = useAction((name: string, value: string) => putTargetKeySecret(scope, name, value));

  const HandleSubmit = () => {
    const field = valueRef.current;
    const value = field?.value ?? '';
    if (value.trim() === '' || !nameOk) return;
    void save.Run(Name, value).then((result) => {
      if (isApiError(result)) return;
      if (field) field.value = '';
      setEmpty(true);
      Push({ Id: `target-key-secret-${Name}`, Accent: 'live', Title: t('targets.keySecret.saved', { name: Name }) });
      if (nameFixed) refresh.Changed();
      else setAttach(Name);
    });
  };

  return (
    <div className="grid gap-3">
      <form
        noValidate
        className="grid gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          HandleSubmit();
        }}
      >
        {save.Failure ? <ErrorAlert error={save.Failure} focus /> : null}
        <Field label={t('targets.keySecret.name')}>
          <Input
            dir="ltr"
            className="font-mono"
            maxLength={64}
            autoComplete="off"
            spellCheck={false}
            readOnly={nameFixed}
            disabled={save.Pending || Attach !== null}
            value={Name}
            onChange={(event) => {
              setName(event.target.value);
            }}
          />
        </Field>
        <Field label={t('targets.keySecret.value')}>
          <Textarea
            ref={valueRef}
            dir="ltr"
            rows={5}
            className="font-mono"
            spellCheck={false}
            autoComplete="off"
            disabled={save.Pending || Attach !== null}
            onChange={(event) => {
              setEmpty(event.target.value.trim() === '');
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('targets.keySecret.hint')}</small>
        <Button
          type="submit"
          size="sm"
          variant="primary"
          iconStart="key"
          className="justify-self-start"
          disabled={save.Pending || Empty || !nameOk || Attach !== null}
        >
          {save.Pending ? t('targets.keySecret.saving') : t('targets.keySecret.submit')}
        </Button>
      </form>
      {Attach !== null ? (
        <TargetFactorPanel
          question={t('targets.keySecret.attach.question', { name: Attach })}
          confirm={t('targets.keySecret.attach.confirm')}
          icon="check"
          run={(factor) => updateTarget(scope, target.Name, target.Version, { KeySecretName: Attach }, factor)}
          onDone={() => {
            setAttach(null);
            refresh.Changed();
          }}
          onCancel={() => {
            setAttach(null);
          }}
        />
      ) : null}
    </div>
  );
};

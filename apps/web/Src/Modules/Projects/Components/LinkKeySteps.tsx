import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TRepository } from '@/Modules/Projects/Api/ProjectsApi';
import { CopyField } from '@/Modules/Projects/Components/CopyField';
import { useConfirmHostKey } from '@/Modules/Projects/Hooks/useRepository';
import { EXAMPLES } from '@/Modules/Projects/Types/LinkForm';

export interface IDeployKeyStepProps {
  repository: TRepository;
  onNext: () => void;
}

export const DeployKeyStep = ({ repository, onNext }: IDeployKeyStepProps) => {
  const t = useT();
  return (
    <div className="grid gap-5">
      <p className="m-0 text-body-sm text-secondary">{t('repository.deployKey.lead')}</p>
      {repository.DeployKeyPublic === undefined ? null : (
        <CopyField
          id="deploy-key"
          label={t('repository.deployKey.label')}
          value={repository.DeployKeyPublic}
          copy={t('repository.deployKey.copy')}
          copied={t('repository.deployKey.copied')}
        />
      )}
      <ol className="m-0 grid list-decimal gap-2 ps-5 text-body-sm text-secondary">
        <li>{t('repository.deployKey.how.open', { menu: EXAMPLES.deployKeysMenu })}</li>
        <li>{t('repository.deployKey.how.paste')}</li>
        <li>{t('repository.deployKey.how.readOnly', { option: EXAMPLES.writeOption })}</li>
      </ol>
      <InlineAlert accent="awaiting" title={t('repository.deployKey.readOnly.title')}>
        {t('repository.deployKey.readOnly.body')}
      </InlineAlert>
      <Button variant="primary" iconStart="next" className="justify-self-start" onClick={onNext}>
        {t('repository.deployKey.next')}
      </Button>
    </div>
  );
};

export interface IHostKeyStepProps {
  organizationId: string;
  projectId: string;
  repository: TRepository;
  onPinned: (repository: TRepository) => void;
  onBack: () => void;
}

export const HostKeyStep = ({ organizationId, projectId, repository, onPinned, onBack }: IHostKeyStepProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const confirm = useConfirmHostKey(organizationId, projectId);
  const [Checked, setChecked] = useState(false);
  const pending = repository.PendingHostKeyFingerprint;

  if (pending === undefined) {
    return (
      <div className="grid gap-5">
        <InlineAlert accent="live" title={t('repository.hostKey.pinned.title')}>
          {t('repository.hostKey.pinned.body')}
        </InlineAlert>
        {repository.HostKeyFingerprint === undefined ? null : (
          <code dir="ltr" className="block rounded-md border border-line bg-surface-inset px-3 py-2.5 text-code break-all">
            {repository.HostKeyFingerprint}
          </code>
        )}
        <Button variant="primary" iconStart="next" className="justify-self-start" onClick={() => { onPinned(repository); }}>
          {t('repository.hostKey.next')}
        </Button>
      </div>
    );
  }

  return (
    <div className="grid gap-5">
      <p className="m-0 text-body-sm text-secondary">{t('repository.hostKey.lead')}</p>
      <div className="grid gap-2">
        <span className="text-body-sm text-secondary">{t('repository.hostKey.fingerprint')}</span>
        <code dir="ltr" aria-label={t('repository.hostKey.fingerprint')} className="block rounded-md border border-line-2 bg-surface-inset px-3 py-2.5 text-code break-all select-all">
          {pending}
        </code>
      </div>
      <ol className="m-0 grid list-decimal gap-2 ps-5 text-body-sm text-secondary">
        <li>{t('repository.hostKey.how.ask')}</li>
        <li>
          {t('repository.hostKey.how.command')}
          <code dir="ltr" className="mt-1.5 block rounded-md border border-line bg-surface-inset px-3 py-2 text-code break-all">
            {EXAMPLES.hostKeyCommand}
          </code>
        </li>
        <li>{t('repository.hostKey.how.compare')}</li>
      </ol>
      {repository.HostKeyFingerprint === undefined ? null : (
        <InlineAlert accent="failed" title={t('repository.hostKey.changed.title')}>
          {t('repository.hostKey.changed.body', { old: repository.HostKeyFingerprint })}
        </InlineAlert>
      )}
      {confirm.error ? <InlineAlert accent="failed" title={errorText(confirm.error)} /> : null}
      <label className="flex items-start gap-2.5 text-body-sm text-primary">
        <input
          type="checkbox"
          checked={Checked}
          className="size-6 flex-none accent-accent"
          onChange={(event) => {
            setChecked(event.target.checked);
          }}
        />
        {t('repository.hostKey.checked')}
      </label>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="primary"
          iconStart="shield"
          disabled={!Checked || confirm.isPending}
          onClick={() => {
            confirm.mutate(pending, { onSuccess: onPinned });
          }}
        >
          {confirm.isPending ? t('repository.hostKey.confirming') : t('repository.hostKey.confirm')}
        </Button>
        <Button variant="ghost" disabled={confirm.isPending} onClick={onBack}>
          {t('repository.hostKey.back')}
        </Button>
      </div>
    </div>
  );
};

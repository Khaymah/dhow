import { useState } from 'react';

import { isApiError } from '@/Core/Api/ApiError';
import { useAction } from '@/Core/Hooks/useAction';
import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useRelativeTime } from '@/Core/Hooks/useRelativeTime';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Field } from '@/Core/Ui/Field';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Textarea } from '@/Core/Ui/Textarea';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { confirmHostKey, fetchHostKey, pasteHostKey, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { CheckField } from '@/Modules/Environments/Components/CheckField';
import { TargetFactorPanel } from '@/Modules/Environments/Components/TargetFactorPanel';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';

type THost = TTarget['Hosts'][number];

export interface IHostKeyFlowProps {
  scope: ITargetScope;
  target: TTarget;
  host: THost;
  canManage: boolean;
}

const Fingerprint = ({ value, label }: { value: string; label: string }) => (
  <p className="m-0 grid gap-0.5 text-body-sm">
    <span className="text-secondary">{label}</span>
    <bdi dir="ltr" className="font-mono break-all text-primary">
      {value}
    </bdi>
  </p>
);

const PasteForm = ({ scope, target, host, onDone }: IHostKeyFlowProps & { onDone: () => void }) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const [Text, setText] = useState('');
  const paste = useAction((text: string) => pasteHostKey(scope, target.Name, host.Id, text));
  const errorText = useApiErrorText();

  return (
    <form
      noValidate
      className="grid gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        void paste.Run(Text).then((result) => {
          if (isApiError(result)) return;
          setText('');
          refresh.Changed();
          onDone();
        });
      }}
    >
      {paste.Failure ? <InlineAlert accent="failed" title={errorText(paste.Failure)} /> : null}
      <Field label={t('targets.hostkey.paste.label')}>
        <Textarea
          dir="ltr"
          rows={3}
          className="font-mono"
          spellCheck={false}
          autoComplete="off"
          disabled={paste.Pending}
          value={Text}
          onChange={(event) => {
            setText(event.target.value);
          }}
        />
      </Field>
      <small className="text-body-sm text-tertiary">{t('targets.hostkey.paste.hint')}</small>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" variant="primary" iconStart="check" disabled={paste.Pending || Text.trim() === ''}>
          {t('targets.hostkey.paste.submit')}
        </Button>
        <Button size="sm" variant="ghost" disabled={paste.Pending} onClick={onDone}>
          {t('mfa.cancel')}
        </Button>
      </div>
    </form>
  );
};

const PendingConfirm = ({ scope, target, host }: IHostKeyFlowProps) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const Push = useToastStore((state) => state.Push);
  const [Checked, setChecked] = useState(false);
  const pending = host.PendingHostKeyFingerprint ?? '';

  return (
    <div className="grid gap-3 rounded-md border border-warn-line bg-warn-wash p-card">
      <p className="m-0 text-label">{t('targets.hostkey.pending.title')}</p>
      <Fingerprint value={pending} label={t('targets.hostkey.pending.fingerprint')} />
      <p className="m-0 text-body-sm text-secondary">{t('targets.hostkey.pending.compare')}</p>
      <CheckField
        label={t('targets.hostkey.pending.check')}
        about={t('targets.hostkey.pending.check.about')}
        checked={Checked}
        disabled={false}
        onChange={setChecked}
      />
      <TargetFactorPanel
        confirm={t('targets.hostkey.pending.confirm')}
        question={t('targets.hostkey.pending.question', { host: `${host.Host}:${String(host.Port)}` })}
        icon="shield"
        disabled={!Checked}
        run={(factor) => confirmHostKey(scope, target.Name, host.Id, pending, factor)}
        onDone={() => {
          setChecked(false);
          refresh.Changed();
          Push({ Id: `host-key-${host.Id}`, Accent: 'live', Title: t('targets.hostkey.pinned.toast', { host: host.Host }) });
        }}
        onCancel={() => {
          setChecked(false);
        }}
      />
    </div>
  );
};

export const HostKeyFlow = (props: IHostKeyFlowProps) => {
  const { scope, target, host, canManage } = props;
  const t = useT();
  const time = useRelativeTime();
  const errorText = useApiErrorText();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const [Pasting, setPasting] = useState(false);
  const fetched = useAction(() => fetchHostKey(scope, target.Name, host.Id));

  const HandleFetch = () => {
    void fetched.Run().then((result) => {
      if (!isApiError(result)) {
        refresh.Changed();
        return;
      }
      if (result.Code === 'TARGET_HOST_BLOCKED') setPasting(true);
    });
  };

  return (
    <div className="grid gap-3 px-card pb-3">
      {host.KeyPinned && host.HostKeyFingerprint ? (
        <div className="grid gap-1">
          <Fingerprint value={host.HostKeyFingerprint} label={t('targets.hostkey.pinned')} />
          {host.HostKeyConfirmedAt ? (
            <small className="text-body-sm text-tertiary">{t('targets.hostkey.confirmedAt', { time: time.ago(host.HostKeyConfirmedAt) })}</small>
          ) : null}
        </div>
      ) : (
        <InlineAlert accent="awaiting" title={t('targets.hostkey.unpinned')} />
      )}
      {fetched.Failure ? <InlineAlert accent="failed" title={errorText(fetched.Failure)} /> : null}
      {host.PendingHostKeyFingerprint ? <PendingConfirm key={host.PendingHostKeyFingerprint} {...props} /> : null}
      {canManage && !Pasting ? (
        <div className="flex flex-wrap gap-2">
          <Button size="sm" iconStart="download" disabled={fetched.Pending} onClick={HandleFetch}>
            {fetched.Pending ? t('targets.hostkey.fetching') : t('targets.hostkey.fetch')}
          </Button>
          <Button
            size="sm"
            iconStart="key"
            onClick={() => {
              setPasting(true);
            }}
          >
            {t('targets.hostkey.paste')}
          </Button>
        </div>
      ) : null}
      {canManage && Pasting ? (
        <PasteForm
          {...props}
          onDone={() => {
            setPasting(false);
          }}
        />
      ) : null}
    </div>
  );
};

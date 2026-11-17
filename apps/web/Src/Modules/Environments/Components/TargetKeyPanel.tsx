import { useState } from 'react';

import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { copyText } from '@/Core/Ui/Clipboard';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { useT } from '@/I18n/useT';
import type { TTarget } from '@/Modules/Environments/Api/TargetsApi';
import { generateTargetKey, type ITargetScope } from '@/Modules/Environments/Api/TargetWritesApi';
import { KeySecretForm } from '@/Modules/Environments/Components/KeySecretForm';
import { TargetFactorPanel } from '@/Modules/Environments/Components/TargetFactorPanel';
import { useTargetRefresh } from '@/Modules/Environments/Hooks/useTargetRefresh';

export interface ITargetKeyPanelProps {
  scope: ITargetScope;
  target: TTarget;
  canManage: boolean;
}

const PublicKey = ({ value }: { value: string }) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);

  return (
    <div className="grid gap-2">
      <p className="m-0 text-body-sm text-secondary">{t('targets.key.public')}</p>
      <pre className="m-0 rounded-md border border-line bg-wash p-3 text-body-sm whitespace-pre-wrap break-all">
        <bdi dir="ltr" className="font-mono break-all whitespace-pre-wrap">
          {value}
        </bdi>
      </pre>
      <Button
        size="sm"
        iconStart="copy"
        className="justify-self-start"
        onClick={() => {
          void copyText(value).then((copied) => {
            if (copied) Push({ Id: 'target-public-key-copied', Accent: 'live', Title: t('targets.key.copied') });
          });
        }}
      >
        {t('targets.key.copy')}
      </Button>
      <small className="text-body-sm text-tertiary">{t('targets.key.public.hint')}</small>
    </div>
  );
};

export const TargetKeyPanel = ({ scope, target, canManage }: ITargetKeyPanelProps) => {
  const t = useT();
  const refresh = useTargetRefresh(scope.OrganizationId, scope.ProjectId, scope.EnvironmentName);
  const [Generating, setGenerating] = useState(false);
  const missing = target.NotReady.includes('key_missing');

  return (
    <div className="grid gap-4 p-card">
      {missing ? <InlineAlert accent="awaiting" title={t('targets.key.missing')} /> : null}
      <p className="m-0 text-body-sm text-secondary">
        {target.KeySecretName ? t('targets.key.secret', { name: target.KeySecretName }) : t('targets.key.noSecret')}
      </p>
      {target.PublicKey ? <PublicKey value={target.PublicKey} /> : null}
      {canManage ? (
        <>
          {Generating ? (
            <TargetFactorPanel
              question={t(target.PublicKey ? 'targets.key.generate.again' : 'targets.key.generate.question')}
              confirm={t('targets.key.generate.confirm')}
              icon="key"
              run={(factor) => generateTargetKey(scope, target.Name, factor)}
              onDone={() => {
                setGenerating(false);
                refresh.Changed();
              }}
              onCancel={() => {
                setGenerating(false);
              }}
            />
          ) : (
            <Button
              size="sm"
              iconStart="key"
              className="justify-self-start"
              onClick={() => {
                setGenerating(true);
              }}
            >
              {target.PublicKey ? t('targets.key.generate.rotate') : t('targets.key.generate')}
            </Button>
          )}
          <div className="grid gap-3 border-t border-line pt-4">
            <h4 className="m-0 text-label">{t('targets.keySecret.title')}</h4>
            <KeySecretForm scope={scope} target={target} />
          </div>
        </>
      ) : null}
    </div>
  );
};

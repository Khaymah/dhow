import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import type { TOrganization } from '@/Core/Auth/AuthTypes';
import type { TCurrentFactor } from '@/Core/Auth/Factor';
import { useFactorMethods, useMfaStatus } from '@/Core/Auth/useMfaStatus';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { ConfirmPanel } from '@/Core/Ui/ConfirmPanel';
import { Field } from '@/Core/Ui/Field';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { Skeleton } from '@/Core/Ui/Skeleton';
import type { TTranslate } from '@/I18n/I18nContext';
import { useT } from '@/I18n/useT';
import { changeLogRetention } from '@/Modules/Organization/Api/OrganizationApi';
import { organizationKey } from '@/Modules/Organization/Hooks/useOrganization';

const RETENTION_MIN_DAYS = 7;
const RETENTION_MAX_DAYS = 3650;

const parseDays = (raw: string): number | null | undefined => {
  const value = raw.trim();
  if (value === '') return null;
  if (!/^\d+$/.test(value)) return undefined;
  const days = Number(value);
  return days >= RETENTION_MIN_DAYS && days <= RETENTION_MAX_DAYS ? days : undefined;
};

const currentText = (organization: TOrganization, t: TTranslate): string =>
  organization.LogRetentionDays === undefined ? t('logRetention.current.server') : t('logRetention.current.days', { days: organization.LogRetentionDays });

export const LogRetentionSummary = ({ organization }: { organization: TOrganization }) => {
  const t = useT();
  return (
    <Card title={t('logRetention.title')} icon="file" padded>
      <div className="grid gap-1.5">
        <p className="m-0 text-body text-primary">{currentText(organization, t)}</p>
        <p className="m-0 text-body-sm text-secondary">{t('logRetention.about')}</p>
        <p className="m-0 text-body-sm text-tertiary">{t('logRetention.ownerOnly')}</p>
      </div>
    </Card>
  );
};

export const LogRetentionCard = ({ organization }: { organization: TOrganization }) => {
  const t = useT();
  const client = useQueryClient();
  const status = useMfaStatus();
  const methods = useFactorMethods();
  const Push = useToastStore((state) => state.Push);
  const [Raw, setRaw] = useState(organization.LogRetentionDays === undefined ? '' : String(organization.LogRetentionDays));
  const [Confirming, setConfirming] = useState(false);
  const [Problem, setProblem] = useState<string | undefined>(undefined);
  const days = parseDays(Raw);
  const action = useFactorAction(methods ?? ['password'], async (proof: () => Promise<TCurrentFactor>) =>
    changeLogRetention(organization.Id, days ?? null, await proof()),
  );
  const dirty = days !== undefined && days !== (organization.LogRetentionDays ?? null);

  if (status.data === undefined || methods === undefined) {
    return (
      <Card title={t('logRetention.title')} icon="file" padded>
        <Skeleton label={t('ui.loading')} lines={3} />
      </Card>
    );
  }

  const HandleReview = () => {
    if (days === undefined) {
      setProblem(t('logRetention.error.range', { min: RETENTION_MIN_DAYS, max: RETENTION_MAX_DAYS }));
      return;
    }
    setProblem(undefined);
    setConfirming(true);
  };

  const HandleConfirm = () => {
    void action.Run().then((saved) => {
      if (!saved) return;
      client.setQueryData(organizationKey(organization.Id), saved);
      Push({ Id: `log-retention-${saved.Id}`, Accent: 'live', Title: t('logRetention.saved') });
      setConfirming(false);
      action.Reset();
    });
  };

  return (
    <Card title={t('logRetention.title')} icon="file">
      <div className="grid gap-3 px-card py-3">
        <p className="m-0 text-body-sm text-secondary">{t('logRetention.about')}</p>
        <p className="m-0 text-body-sm text-primary">{currentText(organization, t)}</p>
        {status.data.Enabled ? (
          <form
            noValidate
            className="grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              HandleReview();
            }}
          >
            <Field label={t('logRetention.field')} error={Problem}>
              <Input
                type="number"
                inputMode="numeric"
                min={RETENTION_MIN_DAYS}
                max={RETENTION_MAX_DAYS}
                className="max-w-40"
                value={Raw}
                disabled={Confirming}
                onChange={(event) => {
                  setRaw(event.target.value);
                  setConfirming(false);
                }}
              />
            </Field>
            <small className="text-body-sm text-tertiary">{t('logRetention.hint', { min: RETENTION_MIN_DAYS, max: RETENTION_MAX_DAYS })}</small>
            {Confirming ? null : (
              <Button type="submit" size="sm" className="justify-self-start" disabled={!dirty && days !== undefined}>
                {t('logRetention.review')}
              </Button>
            )}
          </form>
        ) : (
          <InlineAlert accent="awaiting" title={t('logRetention.ownFirst')} />
        )}
      </div>
      {Confirming && days !== undefined ? (
        <ConfirmPanel
          question={days === null ? t('logRetention.question.server') : t('logRetention.question.days', { days })}
          confirm={t('logRetention.confirm')}
          icon="file"
          methods={methods}
          action={action}
          onConfirm={HandleConfirm}
          onCancel={() => {
            setConfirming(false);
            action.Reset();
          }}
        />
      ) : null}
    </Card>
  );
};

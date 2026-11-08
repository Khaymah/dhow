import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { CheckList } from '@/Core/Ui/CheckList';
import { Field } from '@/Core/Ui/Field';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { useT } from '@/I18n/useT';
import { putRule, type TNotificationChannel, type INotificationRule } from '@/Modules/Notifications/Api/NotificationsApi';
import { useNotificationsRefresh } from '@/Modules/Notifications/Hooks/useNotifications';
import {
  eventLabel,
  EVENTS,
  ruleInputOf,
  ruleProblem,
  RULE_ROLES,
  sendsByRole,
  statusLabel,
  STATUSES,
  type IRuleDraft,
} from '@/Modules/Notifications/Types/NotificationsForm';

export interface IRulesEditorProps {
  organizationId: string;
  projectId: string;
  channel: TNotificationChannel;
  rule: INotificationRule | undefined;
  writable: boolean;
  onClose: () => void;
}

const draftOf = (rule: INotificationRule | undefined): IRuleDraft => ({
  Events: rule?.Events ?? [],
  Statuses: rule?.Statuses ?? [],
  Roles: RULE_ROLES.filter((role) => rule?.Roles.includes(role) === true),
  Environments: (rule?.Environments ?? []).join(', '),
});

export const RulesEditor = ({ organizationId, projectId, channel, rule, writable, onClose }: IRulesEditorProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const refresh = useNotificationsRefresh(organizationId, projectId);
  const Push = useToastStore((state) => state.Push);
  const [Draft, setDraft] = useState<IRuleDraft>(draftOf(rule));
  const [Problem, setProblem] = useState<string | null>(null);
  const [Busy, setBusy] = useState(false);

  const HandleSave = () => {
    const problem = ruleProblem(channel.Kind, Draft);
    if (problem !== null) {
      setProblem(t(problem));
      return;
    }
    setProblem(null);
    setBusy(true);
    putRule(organizationId, projectId, ruleInputOf(channel.Id, channel.Kind, Draft))
      .then(async () => {
        await refresh.Changed();
        Push({ Id: `notification-rules-${channel.Id}`, Accent: 'live', Title: t('notifications.rules.saved', { name: channel.Name }) });
        onClose();
      })
      .catch((error: unknown) => {
        setProblem(errorText(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <form
      noValidate
      aria-label={t('notifications.rules.title', { name: channel.Name })}
      className="grid gap-4 px-card pb-card"
      onSubmit={(event) => {
        event.preventDefault();
        HandleSave();
      }}
    >
      {writable ? null : <InlineAlert accent="building" title={t('notifications.rules.readOnly')} />}
      {Problem ? <InlineAlert focus accent="failed" title={Problem} /> : null}
      <CheckList
        label={t('notifications.rules.events')}
        disabled={!writable || Busy}
        values={Draft.Events}
        items={EVENTS.map((event) => ({ value: event, title: t(eventLabel(event)) }))}
        onChange={(Events) => {
          setDraft({ ...Draft, Events });
        }}
      />
      <CheckList
        label={t('notifications.rules.statuses')}
        disabled={!writable || Busy}
        values={Draft.Statuses}
        items={STATUSES.map((status) => ({ value: status, title: t(statusLabel(status)) }))}
        onChange={(Statuses) => {
          setDraft({ ...Draft, Statuses });
        }}
      />
      <div className="grid gap-1.5">
        <Field label={t('notifications.rules.environments')}>
          <Input
            dir="ltr"
            autoComplete="off"
            disabled={!writable || Busy}
            value={Draft.Environments}
            onChange={(event) => {
              setDraft({ ...Draft, Environments: event.target.value });
            }}
          />
        </Field>
        <small className="text-body-sm text-tertiary">{t('notifications.rules.environments.hint')}</small>
      </div>
      {sendsByRole(channel.Kind) ? (
        <CheckList
          label={t('notifications.rules.roles')}
          disabled={!writable || Busy}
          values={Draft.Roles}
          items={RULE_ROLES.map((role) => ({ value: role, title: t(`role.${role}`) }))}
          onChange={(Roles) => {
            setDraft({ ...Draft, Roles });
          }}
        />
      ) : null}
      <div className="flex flex-wrap gap-2">
        {writable ? (
          <Button type="submit" size="sm" variant="primary" iconStart="check" disabled={Busy}>
            {Busy ? t('notifications.rules.saving') : t('notifications.rules.save')}
          </Button>
        ) : null}
        <Button size="sm" variant="ghost" disabled={Busy} onClick={onClose}>
          {t('notifications.rules.close')}
        </Button>
      </div>
    </form>
  );
};

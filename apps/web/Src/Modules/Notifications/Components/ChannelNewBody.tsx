import { useSearch } from '@tanstack/react-router';
import { useState } from 'react';

import type { TRole } from '@/Core/Auth/AuthTypes';
import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { useFactorAction } from '@/Core/Hooks/useFactorAction';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import { createChannel, type TCreatedNotificationChannel } from '@/Modules/Notifications/Api/NotificationsApi';
import { ChannelFields } from '@/Modules/Notifications/Components/ChannelFields';
import { FactorSubmit } from '@/Modules/Notifications/Components/FactorSubmit';
import { SecretCreated } from '@/Modules/Notifications/Components/SecretCreated';
import { useNotificationsRefresh } from '@/Modules/Notifications/Hooks/useNotifications';
import { canManageNotifications } from '@/Modules/Notifications/Types/NotificationsAccess';
import { createInputOf, draftProblem, EMPTY_CHANNEL_DRAFT, type IChannelDraft } from '@/Modules/Notifications/Types/NotificationsForm';

export interface IChannelNewBodyProps {
  projectId?: string;
  role?: TRole | undefined;
  active?: boolean | undefined;
  onDone: () => void;
}

const SLIDE_ID = 'notification-channel-new';

interface INewFormProps {
  organizationId: string;
  projectId: string;
  methods: readonly TFactorMethod[];
  onDone: () => void;
}

const NewForm = ({ organizationId, projectId, methods, onDone }: INewFormProps) => {
  const t = useT();
  const refresh = useNotificationsRefresh(organizationId, projectId);
  const [Draft, setDraft] = useState<IChannelDraft>(EMPTY_CHANNEL_DRAFT);
  const [Problem, setProblem] = useState<string | null>(null);
  const [Secret, setSecret] = useState<string | null>(null);
  const action = useFactorAction<TCreatedNotificationChannel>(methods, async (proof) =>
    createChannel(organizationId, projectId, createInputOf(Draft), await proof()),
  );

  if (Secret !== null) {
    return (
      <SecretCreated
        secret={Secret}
        onDone={() => {
          setSecret(null);
          onDone();
        }}
      />
    );
  }

  const HandleSubmit = () => {
    const problem = draftProblem(Draft);
    if (problem !== null) {
      setProblem(t(problem));
      return;
    }
    setProblem(null);
    void action.Run().then(async (created) => {
      if (created === null) return;
      await refresh.Changed();
      setDraft(EMPTY_CHANNEL_DRAFT);
      if (created.Secret === undefined) onDone();
      else setSecret(created.Secret);
    });
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
      {Problem ? <InlineAlert accent="failed" title={Problem} /> : null}
      {action.Notice ? <InlineAlert accent="failed" title={action.Notice} /> : null}
      <ChannelFields
        draft={Draft}
        disabled={action.Busy}
        onChange={(next) => {
          setDraft(next);
          setProblem(null);
        }}
      />
      <FactorSubmit methods={methods} action={action} label={t('notifications.form.submit')} busyLabel={t('notifications.form.submitting')} />
    </form>
  );
};

export const ChannelNewBody = ({ projectId, role, active, onDone }: IChannelNewBodyProps) => {
  const t = useT();
  const membership = useCurrentMembership();
  const { slide } = useSearch({ strict: false });
  if (slide !== SLIDE_ID) return null;
  if (membership === undefined || (projectId !== undefined && active === undefined)) {
    return <Skeleton label={t('ui.loading')} lines={5} />;
  }
  if (projectId === undefined) return <InlineAlert accent="awaiting" title={t('notifications.form.noProject')} />;
  if (!canManageNotifications(role)) return <InlineAlert accent="failed" title={t('notifications.forbidden.manage')} />;
  if (!active) return <InlineAlert accent="building" title={t('projects.archived.blocked')} />;
  return (
    <FactorGate>
      {(methods) => <NewForm organizationId={membership.OrganizationId} projectId={projectId} methods={methods} onDone={onDone} />}
    </FactorGate>
  );
};

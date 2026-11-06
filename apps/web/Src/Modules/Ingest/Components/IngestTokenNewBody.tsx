import { useState } from 'react';

import type { TFactorMethod } from '@/Core/Auth/Factor';
import { useCurrentMembership } from '@/Core/Auth/useMe';
import { FactorGate } from '@/Core/Ui/FactorGate';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TCreatedIngestToken } from '@/Modules/Ingest/Api/IngestApi';
import { IngestFactorSubmit } from '@/Modules/Ingest/Components/IngestFactorSubmit';
import { IngestReachConfirm } from '@/Modules/Ingest/Components/IngestReachConfirm';
import { IngestTokenCreated } from '@/Modules/Ingest/Components/IngestTokenCreated';
import { IngestTokenFields } from '@/Modules/Ingest/Components/IngestTokenFields';
import { useCreateIngestToken } from '@/Modules/Ingest/Hooks/useCreateIngestToken';
import { useIngestRefresh } from '@/Modules/Ingest/Hooks/useIngest';
import { draftProblem, EMPTY_TOKEN_DRAFT, type ITokenDraft } from '@/Modules/Ingest/Types/IngestForm';

export interface IIngestTokenNewBodyProps {
  projectId?: string;
  onDone: () => void;
}

interface INewFormProps {
  organizationId: string;
  projectId: string;
  methods: readonly TFactorMethod[];
  onDone: () => void;
}

const NewForm = ({ organizationId, projectId, methods, onDone }: INewFormProps) => {
  const t = useT();
  const refresh = useIngestRefresh(organizationId, projectId);
  const [Draft, setDraft] = useState<ITokenDraft>(EMPTY_TOKEN_DRAFT);
  const [Problem, setProblem] = useState<string | null>(null);
  const [Reach, setReach] = useState<string[] | null>(null);
  const [Acknowledged, setAcknowledged] = useState(false);
  const [Created, setCreated] = useState<TCreatedIngestToken | null>(null);

  const action = useCreateIngestToken(methods, {
    OrganizationId: organizationId,
    ProjectId: projectId,
    Draft,
    Acknowledged,
    OnReach: setReach,
  });

  if (Created !== null) {
    return (
      <IngestTokenCreated
        created={Created}
        onDone={() => {
          setCreated(null);
          onDone();
        }}
      />
    );
  }

  const HandleChange = (next: ITokenDraft) => {
    setDraft(next);
    setProblem(null);
    if (Reach !== null) {
      setReach(null);
      setAcknowledged(false);
      action.Reset();
    }
  };

  const HandleSubmit = () => {
    const problem = draftProblem(Draft);
    if (problem !== null) {
      setProblem(t(problem));
      return;
    }
    if (Reach !== null && !Acknowledged) {
      setProblem(t('ingest.reach.needAck'));
      return;
    }
    setProblem(null);
    void action.Run().then(async (created) => {
      if (created === null) return;
      setReach(null);
      await refresh.Changed();
      setCreated(created);
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
      {action.Notice && Reach === null ? <InlineAlert accent="failed" title={action.Notice} /> : null}
      <IngestTokenFields draft={Draft} disabled={action.Busy} onChange={HandleChange} />
      {Reach !== null ? (
        <IngestReachConfirm
          names={Reach}
          acknowledged={Acknowledged}
          disabled={action.Busy}
          onChange={(value) => {
            setAcknowledged(value);
            setProblem(null);
          }}
        />
      ) : null}
      <IngestFactorSubmit
        methods={methods}
        action={action}
        label={t('ingest.form.submit')}
        busyLabel={t('ingest.form.submitting')}
        disabled={Reach !== null && !Acknowledged}
      />
    </form>
  );
};

export const IngestTokenNewBody = ({ projectId, onDone }: IIngestTokenNewBodyProps) => {
  const t = useT();
  const membership = useCurrentMembership();
  if (membership === undefined) return <Skeleton label={t('ui.loading')} lines={5} />;
  if (projectId === undefined) return <InlineAlert accent="awaiting" title={t('ingest.form.noProject')} />;
  return (
    <FactorGate>
      {(methods) => <NewForm organizationId={membership.OrganizationId} projectId={projectId} methods={methods} onDone={onDone} />}
    </FactorGate>
  );
};

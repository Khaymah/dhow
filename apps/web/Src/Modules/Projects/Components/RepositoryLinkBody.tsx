import { useEffect, useState } from 'react';

import { useSlideGuardStore } from '@/Core/Store/useSlideGuardStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { EmptyState } from '@/Core/Ui/EmptyState';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Skeleton } from '@/Core/Ui/Skeleton';
import { useT } from '@/I18n/useT';
import type { TRepository, TRepositoryWithSecret } from '@/Modules/Projects/Api/ProjectsApi';
import { DeployKeyStep, HostKeyStep } from '@/Modules/Projects/Components/LinkKeySteps';
import { StatusStep, WebhookStep } from '@/Modules/Projects/Components/LinkFinalSteps';
import { LinkSourceStep } from '@/Modules/Projects/Components/LinkSourceStep';
import { LinkStepper, stepsFor, type TLinkStep } from '@/Modules/Projects/Components/LinkStepper';
import { useOrganizationAccess } from '@/Modules/Projects/Hooks/useProjectAccess';
import { useProject } from '@/Modules/Projects/Hooks/useProjects';
import { isNotLinked, useRepository, useRepositorySaved } from '@/Modules/Projects/Hooks/useRepository';
import { canOnProject } from '@/Modules/Projects/Types/ProjectAccess';

export interface IRepositoryLinkBodyProps {
  projectId: string | undefined;
  onDone: () => void;
}

interface IFlowProps {
  organizationId: string;
  projectId: string;
  current: TRepository | undefined;
  onDone: () => void;
}

const firstStep = (current: TRepository | undefined): TLinkStep => {
  if (current === undefined) return 'source';
  if (current.PendingHostKeyFingerprint !== undefined) return 'host-key';
  return current.LastDeliveryAt === undefined ? 'webhook' : 'source';
};

const useSecretGuard = (guarded: boolean) => {
  const Block = useSlideGuardStore((state) => state.Block);
  const Attempts = useSlideGuardStore((state) => state.Attempts);
  const [Seen, setSeen] = useState(Attempts);
  useEffect(() => {
    Block(guarded);
  }, [Block, guarded]);
  useEffect(
    () => () => {
      Block(false);
    },
    [Block],
  );
  return { Asking: guarded && Attempts !== Seen, Dismiss: () => { setSeen(Attempts); } };
};

const LeaveSecret = ({ onStay, onLeave }: { onStay: () => void; onLeave: () => void }) => {
  const t = useT();
  return (
    <InlineAlert accent="failed" title={t('repository.secret.leave.title')}>
      <span className="grid justify-items-start gap-3">
        {t('repository.secret.leave.body')}
        <span className="flex flex-wrap gap-2">
          <Button variant="primary" iconStart="back" onClick={onStay}>
            {t('repository.secret.leave.stay')}
          </Button>
          <Button variant="danger" iconStart="close" onClick={onLeave}>
            {t('repository.secret.leave.confirm')}
          </Button>
        </span>
      </span>
    </InlineAlert>
  );
};

const LinkFlow = ({ organizationId, projectId, current, onDone }: IFlowProps) => {
  const t = useT();
  const saved = useRepositorySaved(organizationId, projectId);
  const [Step, setStep] = useState<TLinkStep>(() => firstStep(current));
  const [Repository, setRepository] = useState<TRepository | undefined>(current);
  const [Secret, setSecret] = useState<string | null>(null);
  const guard = useSecretGuard(Secret !== null && Step === 'webhook');
  const steps = stepsFor((Repository ?? current)?.Transport === 'ssh' || Step === 'deploy-key' || Step === 'host-key');

  useEffect(
    () => () => {
      setSecret(null);
    },
    [],
  );

  const go = (next: TLinkStep) => {
    if (Step === 'webhook' && next !== 'webhook') setSecret(null);
    setStep(next);
  };

  const HandleLinked = (result: TRepositoryWithSecret) => {
    setRepository(result.Repository);
    saved(result.Repository);
    setSecret(result.WebhookSecret ?? null);
    setStep(result.Repository.Transport === 'ssh' ? 'deploy-key' : 'webhook');
  };

  const HandleLeave = () => {
    setSecret(null);
    guard.Dismiss();
    onDone();
  };

  return (
    <div className="grid gap-6">
      <LinkStepper steps={steps} current={Step} />
      {guard.Asking ? <LeaveSecret onStay={guard.Dismiss} onLeave={HandleLeave} /> : null}
      {Step === 'source' ? <LinkSourceStep organizationId={organizationId} projectId={projectId} current={Repository} onLinked={HandleLinked} /> : null}
      {Step === 'deploy-key' && Repository !== undefined ? <DeployKeyStep repository={Repository} onNext={() => { go('host-key'); }} /> : null}
      {Step === 'host-key' && Repository !== undefined ? (
        <HostKeyStep
          organizationId={organizationId}
          projectId={projectId}
          repository={Repository}
          onPinned={(repository) => {
            setRepository(repository);
            go('webhook');
          }}
          onBack={() => { go('source'); }}
        />
      ) : null}
      {Step === 'webhook' && Repository !== undefined ? (
        <WebhookStep
          organizationId={organizationId}
          projectId={projectId}
          repository={Repository}
          secret={Secret}
          onSecret={setSecret}
          onNext={() => { go('status'); }}
        />
      ) : null}
      {Step === 'status' ? <StatusStep organizationId={organizationId} projectId={projectId} onDone={onDone} /> : null}
      {Step !== 'source' && Repository === undefined ? <InlineAlert accent="failed" title={t('repository.error.lost')} /> : null}
    </div>
  );
};

const Denied = ({ text }: { text: string }) => (
  <Card>
    <EmptyState icon="shield" text={text} />
  </Card>
);

export const RepositoryLinkBody = ({ projectId, onDone }: IRepositoryLinkBodyProps) => {
  const t = useT();
  const access = useOrganizationAccess();
  const organizationId = access.Kind === 'allowed' ? access.OrganizationId : '';
  const project = useProject(organizationId, projectId ?? '');
  const linked = project.data?.RepositoryLinked === true;
  const repository = useRepository(organizationId, projectId ?? '', linked);
  if (projectId === undefined || access.Kind === 'denied') return <Denied text={t('repository.link.missing')} />;
  if (access.Kind === 'loading' || project.isPending || (linked && repository.isPending)) {
    return <Skeleton label={t('ui.loading')} lines={5} />;
  }
  if (project.isError) return <InlineAlert accent="failed" title={t('projects.detail.error')} />;
  const role = project.data.CallerRole ?? access.Role;
  if (!canOnProject(role, 'project.manage')) return <Denied text={t('repository.link.forbidden')} />;
  if (project.data.State === 'archived') return <Denied text={t('repository.link.archived')} />;
  if (repository.isError && !isNotLinked(repository.error)) return <InlineAlert accent="failed" title={t('repository.error.load')} />;
  return <LinkFlow organizationId={organizationId} projectId={projectId} current={linked ? repository.data : undefined} onDone={onDone} />;
};

import { useState } from 'react';

import { useToastStore } from '@/Core/Store/useToastStore';
import { Button } from '@/Core/Ui/Button';
import { Card } from '@/Core/Ui/Card';
import { Field } from '@/Core/Ui/Field';
import { ErrorAlert } from '@/Core/Ui/ErrorAlert';
import { Input } from '@/Core/Ui/Input';
import { useT } from '@/I18n/useT';
import type { TRunner } from '@/Modules/Runners/Api/RunnersApi';
import { HintedField } from '@/Modules/Runners/Components/HintedField';
import { useRevokeRunner, useUpdateRunner } from '@/Modules/Runners/Hooks/useRunners';
import { labelsValid, RESERVED_PREFIX, RUNNER_NAME_MAX, sameLabels, splitLabels } from '@/Modules/Runners/Types/RunnerView';

export interface IRunnerPartProps {
  organizationId: string;
  runner: TRunner;
}

export const RunnerEditCard = ({ organizationId, runner }: IRunnerPartProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const save = useUpdateRunner(organizationId, runner.Id);
  const [Name, setName] = useState(runner.Name);
  const [Labels, setLabels] = useState(runner.Labels.join(', '));
  const [Problem, setProblem] = useState<{ Name?: string; Labels?: string }>({});
  const labels = splitLabels(Labels);
  const dirty = Name.trim() !== runner.Name || !sameLabels(labels, runner.Labels);

  const HandleSubmit = () => {
    const name = Name.trim();
    const next = {
      Name: name === '' || name.length > RUNNER_NAME_MAX ? t('runners.edit.error.name') : undefined,
      Labels: labelsValid(labels) ? undefined : t('runners.new.error.labels', { prefix: RESERVED_PREFIX }),
    };
    setProblem(next);
    if (next.Name !== undefined || next.Labels !== undefined) return;
    save.mutate(
      { Name: name, Labels: labels },
      {
        onSuccess: (updated) => {
          setName(updated.Name);
          setLabels(updated.Labels.join(', '));
          Push({ Id: `runner-saved-${runner.Id}`, Accent: 'live', Title: t('runners.edit.saved') });
        },
      },
    );
  };

  return (
    <Card title={t('runners.edit.title')} icon="settings" padded>
      <form
        noValidate
        className="grid gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          HandleSubmit();
        }}
      >
        {save.error ? <ErrorAlert error={save.error} /> : null}
        <Field label={t('runners.edit.name')} error={Problem.Name}>
          <Input
            maxLength={RUNNER_NAME_MAX}
            autoComplete="off"
            value={Name}
            onChange={(event) => {
              setName(event.target.value);
            }}
          />
        </Field>
        <HintedField label={t('runners.edit.labels')} hint={t('runners.edit.labels.hint', { prefix: RESERVED_PREFIX })} error={Problem.Labels}>
            <Input
              dir="ltr"
              autoComplete="off"
              spellCheck={false}
              value={Labels}
              onChange={(event) => {
                setLabels(event.target.value);
              }}
            />
        </HintedField>
        <Button type="submit" size="sm" variant="primary" className="justify-self-start" disabled={!dirty || save.isPending}>
          {save.isPending ? t('runners.edit.saving') : t('runners.edit.save')}
        </Button>
      </form>
    </Card>
  );
};

export const RunnerPauseCard = ({ organizationId, runner }: IRunnerPartProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const update = useUpdateRunner(organizationId, runner.Id);
  const paused = runner.State === 'disabled';

  return (
    <Card title={paused ? t('runners.resume.title') : t('runners.pause.title')} icon="queue" padded>
      <div className="grid gap-3">
        <p className="m-0 text-body-sm text-secondary">{paused ? t('runners.resume.about') : t('runners.pause.about')}</p>
        {update.error ? <ErrorAlert error={update.error} /> : null}
        <Button
          size="sm"
          className="justify-self-start"
          disabled={update.isPending}
          onClick={() => {
            update.mutate(
              { Disabled: !paused },
              {
                onSuccess: () => {
                  Push({ Id: `runner-paused-${runner.Id}`, Accent: 'live', Title: paused ? t('runners.resume.done') : t('runners.pause.done') });
                },
              },
            );
          }}
        >
          {paused ? t('runners.resume.action') : t('runners.pause.action')}
        </Button>
      </div>
    </Card>
  );
};

export const RunnerRevokeCard = ({ organizationId, runner }: IRunnerPartProps) => {
  const t = useT();
  const Push = useToastStore((state) => state.Push);
  const revoke = useRevokeRunner(organizationId, runner.Id);
  const [Confirming, setConfirming] = useState(false);

  return (
    <Card title={t('runners.revoke.title')} icon="trash" padded>
      <div className="grid gap-3">
        <p className="m-0 text-body-sm text-secondary">{Confirming ? t('runners.revoke.question') : t('runners.revoke.about')}</p>
        {revoke.error ? <ErrorAlert error={revoke.error} /> : null}
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="danger"
            iconStart="trash"
            disabled={revoke.isPending}
            onClick={() => {
              if (!Confirming) {
                setConfirming(true);
                return;
              }
              revoke.mutate(undefined, {
                onSuccess: () => {
                  setConfirming(false);
                  Push({ Id: `runner-revoked-${runner.Id}`, Accent: 'live', Title: t('runners.revoke.done', { name: runner.Name }) });
                },
              });
            }}
          >
            {Confirming ? t('runners.revoke.confirm') : t('runners.revoke.action')}
          </Button>
          {Confirming ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={revoke.isPending}
              onClick={() => {
                setConfirming(false);
              }}
            >
              {t('runners.cancel')}
            </Button>
          ) : null}
        </div>
      </div>
    </Card>
  );
};

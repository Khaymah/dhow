import { useState } from 'react';

import { useApiErrorText } from '@/Core/Hooks/useApiErrorText';
import { Badge } from '@/Core/Ui/Badge';
import { Button } from '@/Core/Ui/Button';
import { InlineAlert } from '@/Core/Ui/InlineAlert';
import { Input } from '@/Core/Ui/Input';
import { useT } from '@/I18n/useT';
import { linkRepository, type TRepository, type TRepositoryWithSecret } from '@/Modules/Projects/Api/ProjectsApi';
import { HintedField } from '@/Modules/Projects/Components/HintedField';
import { useIssueText, type TIssues } from '@/Modules/Projects/Hooks/useIssueText';
import { EXAMPLES, linkFormSchema } from '@/Modules/Projects/Types/LinkForm';

export interface ILinkSourceStepProps {
  organizationId: string;
  projectId: string;
  current: TRepository | undefined;
  onLinked: (result: TRepositoryWithSecret) => void;
}

const COMING = ['github', 'gitlab'] as const;

const Providers = () => {
  const t = useT();
  return (
    <ul aria-label={t('repository.source.providers')} className="m-0 grid list-none gap-2 p-0">
      <li className="grid gap-1 rounded-md border border-accent bg-sel px-4 py-3">
        <span className="flex flex-wrap items-center gap-2 text-label">
          {t('repository.source.generic')}
          <Badge tone="ok" icon="check">
            {t('repository.source.selected')}
          </Badge>
        </span>
        <small className="text-body-sm text-secondary">{t('repository.source.generic.about')}</small>
      </li>
      {COMING.map((provider) => (
        <li key={provider} className="grid gap-1 rounded-md border border-line px-4 py-3">
          <span className="flex flex-wrap items-center gap-2 text-label text-secondary">
            {t(`repository.source.${provider}`)}
            <Badge tone="mute" icon="queue">
              {t('repository.source.coming')}
            </Badge>
          </span>
          <small className="text-body-sm text-tertiary">{t('repository.source.coming.about')}</small>
        </li>
      ))}
    </ul>
  );
};

export const LinkSourceStep = ({ organizationId, projectId, current, onLinked }: ILinkSourceStepProps) => {
  const t = useT();
  const errorText = useApiErrorText();
  const issueText = useIssueText();
  const [CloneUrl, setCloneUrl] = useState(current?.CloneUrl ?? '');
  const [Branch, setBranch] = useState<string>(current?.DefaultBranch ?? EXAMPLES.branch);
  const [Problems, setProblems] = useState<TIssues>({});
  const [ServerError, setServerError] = useState<string | null>(null);
  const [Busy, setBusy] = useState(false);

  const HandleSubmit = () => {
    const parsed = linkFormSchema.safeParse({ CloneUrl, DefaultBranch: Branch });
    if (!parsed.success) {
      setProblems(issueText(parsed.error));
      return;
    }
    setProblems({});
    setServerError(null);
    setBusy(true);
    linkRepository(organizationId, projectId, parsed.data)
      .then(onLinked)
      .catch((error: unknown) => {
        setServerError(errorText(error));
      })
      .finally(() => {
        setBusy(false);
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
      <Providers />
      {current === undefined ? null : <InlineAlert accent="awaiting" title={t('repository.source.relink.title')}>{t('repository.source.relink.body')}</InlineAlert>}
      {ServerError === null ? null : <InlineAlert accent="failed" title={ServerError} />}
      <HintedField label={t('repository.source.url')} hint={t('repository.source.url.hint', { ssh: EXAMPLES.ssh, scp: EXAMPLES.scp })} error={Problems.CloneUrl}>
        <Input
          dir="ltr"
          autoComplete="off"
          spellCheck={false}
          inputMode="url"
          value={CloneUrl}
          disabled={Busy}
          onChange={(event) => {
            setCloneUrl(event.target.value);
          }}
        />
      </HintedField>
      <HintedField label={t('repository.source.branch')} hint={t('repository.source.branch.hint', { branch: EXAMPLES.branch })} error={Problems.DefaultBranch}>
        <Input
          dir="ltr"
          autoComplete="off"
          spellCheck={false}
          value={Branch}
          disabled={Busy}
          className="max-w-64"
          onChange={(event) => {
            setBranch(event.target.value);
          }}
        />
      </HintedField>
      <Button type="submit" variant="primary" iconStart="next" className="justify-self-start" disabled={Busy}>
        {Busy ? t('repository.source.submitting') : current === undefined ? t('repository.source.submit') : t('repository.source.submit.change')}
      </Button>
    </form>
  );
};
